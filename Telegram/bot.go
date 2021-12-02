package wasabeetelegram

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/go-telegram-bot-api/telegram-bot-api"

	"github.com/wasabee-project/Wasabee-Server/auth"
	"github.com/wasabee-project/Wasabee-Server/config"
	"github.com/wasabee-project/Wasabee-Server/generatename"
	"github.com/wasabee-project/Wasabee-Server/log"
	"github.com/wasabee-project/Wasabee-Server/messaging"
	"github.com/wasabee-project/Wasabee-Server/model"
	"github.com/wasabee-project/Wasabee-Server/rocks"
)

// TGConfiguration is the main configuration data for the Telegram interface
// passed to main() pre-loaded with APIKey and TemplateSet set, the rest is built when the bot starts
type TGConfiguration struct {
	APIKey      string
	HookPath    string
	TemplateSet map[string]*template.Template
	baseKbd     tgbotapi.ReplyKeyboardMarkup
	upChan      chan tgbotapi.Update
	hook        string
}

var bot *tgbotapi.BotAPI
var c TGConfiguration

// WasabeeBot is called from main() to start the bot.
func WasabeeBot(init TGConfiguration) {
	if init.APIKey == "" {
		log.Infow("startup", "subsystem", "Telegram", "message", "Telegram API key not set; not starting")
		return
	}
	c.APIKey = init.APIKey

	if init.TemplateSet == nil {
		log.Warnw("startup", "subsystem", "Telegram", "message", "the UI templates are not loaded; not starting Telegram bot")
		return
	}
	c.TemplateSet = init.TemplateSet

	keyboards(&c)

	c.HookPath = init.HookPath
	if c.HookPath == "" {
		c.HookPath = "/tg"
	}

	c.upChan = make(chan tgbotapi.Update, 10) // not using bot.ListenForWebhook() since we need our own bidirectional channel
	webhook := config.Subrouter(c.HookPath)
	webhook.HandleFunc("/{hook}", TGWebHook).Methods("POST")

	messaging.RegisterMessageBus("Telegram", SendMessage)
	messaging.RegisterGroupCalls("Telegram", AddToChat, RemoveFromChat)

	var err error
	bot, err = tgbotapi.NewBotAPI(c.APIKey)
	if err != nil {
		log.Error(err)
		return
	}

	// bot.Debug = true
	log.Infow("startup", "subsystem", "Telegram", "message", "authorized to Telegram as "+bot.Self.UserName)
	config.TGSetBot(bot.Self.UserName, bot.Self.ID)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	webroot := config.GetWebroot()
	c.hook = generatename.GenerateName()
	t := fmt.Sprintf("%s%s/%s", webroot, c.HookPath, c.hook)
	_, err = bot.SetWebhook(tgbotapi.NewWebhook(t))
	if err != nil {
		log.Error(err)
		return
	}

	i := 1
	for update := range c.upChan {
		// log.Debugf("running update: %s", update)
		if err = runUpdate(update); err != nil {
			log.Error(err)
			continue
		}
		if (i % 100) == 0 { // every 100 requests, change the endpoint; I'm _not_ paranoid.
			i = 1
			c.hook = generatename.GenerateName()
			t = fmt.Sprintf("%s%s/%s", webroot, c.HookPath, c.hook)
			_, err = bot.SetWebhook(tgbotapi.NewWebhook(t))
			if err != nil {
				log.Error(err)
			}
		}
		i++
	}
}

// Shutdown closes all the Telegram connections
// called only at server shutdown
func Shutdown() {
	log.Infow("shutdown", "subsystem", "Telegram", "message", "shutdown telegram")
	_, _ = bot.RemoveWebhook()
	bot.StopReceivingUpdates()
}

func runUpdate(update tgbotapi.Update) error {
	if update.CallbackQuery != nil {
		log.Debugw("callback", "subsystem", "Telegram", "data", update)
		msg, err := callback(&update)
		if err != nil {
			log.Error(err)
			return err
		}
		if _, err = bot.Send(msg); err != nil {
			log.Error(err)
			return err
		}
		if _, err = bot.DeleteMessage(tgbotapi.NewDeleteMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID)); err != nil {
			log.Error(err)
			return err
		}
		return nil
	}

	if update.Message != nil {
		if update.Message.Chat.Type == "private" {
			if err := processDirectMessage(&update); err != nil {
				log.Error(err)
			}
		} else {
			if err := processChatMessage(&update); err != nil {
				log.Error(err)
			}
		}
	}

	if update.EditedMessage != nil && update.EditedMessage.Location != nil {
		if err := liveLocationUpdate(&update); err != nil {
			log.Error(err)
		}
	}

	return nil
}

func newUserInit(msg *tgbotapi.MessageConfig, inMsg *tgbotapi.Update) error {
	var ott model.OneTimeToken
	if inMsg.Message.IsCommand() {
		tokens := strings.Split(inMsg.Message.Text, " ")
		if len(tokens) == 2 {
			ott = model.OneTimeToken(strings.TrimSpace(tokens[1]))
		}
	} else {
		ott = model.OneTimeToken(strings.TrimSpace(inMsg.Message.Text))
	}

	tid := model.TelegramID(inMsg.Message.From.ID)
	err := tid.InitAgent(inMsg.Message.From.UserName, ott)
	if err != nil {
		log.Error(err)
		tmp, _ := templateExecute("InitOneFail", inMsg.Message.From.LanguageCode, nil)
		msg.Text = tmp
	} else {
		tmp, _ := templateExecute("InitOneSuccess", inMsg.Message.From.LanguageCode, nil)
		msg.Text = tmp
	}
	return err
}

func newUserVerify(msg *tgbotapi.MessageConfig, inMsg *tgbotapi.Update) error {
	var authtoken string
	if inMsg.Message.IsCommand() {
		tokens := strings.Split(inMsg.Message.Text, " ")
		if len(tokens) == 2 {
			authtoken = tokens[1]
		}
	} else {
		authtoken = inMsg.Message.Text
	}
	authtoken = strings.TrimSpace(authtoken)
	tid := model.TelegramID(inMsg.Message.From.ID)
	err := tid.VerifyAgent(authtoken)
	if err != nil {
		log.Error(err)
		tmp, _ := templateExecute("InitTwoFail", inMsg.Message.From.LanguageCode, nil)
		msg.Text = tmp
	} else {
		tmp, _ := templateExecute("InitTwoSuccess", inMsg.Message.From.LanguageCode, nil)
		msg.Text = tmp
	}
	return err
}

func keyboards(c *TGConfiguration) {
	c.baseKbd = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButtonLocation("Send Location"),
			tgbotapi.NewKeyboardButton("Teams"),
			tgbotapi.NewKeyboardButton("Teammates Nearby"),
		),
		/* -- disable until can be brought up to current
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("My Assignments"),
			tgbotapi.NewKeyboardButton("Nearby Tasks"),
		),
		*/
	)
}

// SendMessage is registered with Wasabee-Server as a message bus to allow other modules to send messages via Telegram
func SendMessage(gid model.GoogleID, message string) (bool, error) {
	tgid, err := gid.TelegramID()
	if err != nil {
		log.Error(err)
		return false, err
	}
	tgid64 := int64(tgid)
	if tgid64 == 0 {
		log.Debugw("TelegramID not found", "subsystem", "Telegram", "GID", gid)
		return false, nil
	}
	msg := tgbotapi.NewMessage(tgid64, "")
	msg.Text = message
	msg.ParseMode = "HTML"

	_, err = bot.Send(msg)
	if err != nil && err.Error() != "Bad Request: chat not found" {
		log.Error(err)
		return false, err
	}
	if err != nil && err.Error() == "Bad Request: chat not found" {
		log.Debugw(err.Error(), "gid", gid, "tgid", tgid)
		return false, nil
	}

	log.Debugw("sent message", "subsystem", "Telegram", "GID", gid)
	return true, nil
}

// checks rocks based on tgid, Inits agent if found
func runRocks(tgid model.TelegramID) (model.GoogleID, error) {
	agent, err := rocks.Search(fmt.Sprint(tgid))
	if err != nil {
		log.Error(err)
		return "", err
	}
	if agent.Gid == "" {
		return "", nil
	}
	gid := model.GoogleID(agent.Gid)

	// add to main tables if necessary
	_, err = auth.Authorize(gid)
	if err != nil {
		log.Error(err)
		return gid, err
	}

	return gid, nil
}
