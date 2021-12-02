package rocks

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/wasabee-project/Wasabee-Server/log"
	"golang.org/x/time/rate"
)

type GoogleID string

// CommunityNotice is sent from a community when an agent is added or removed
// consumed by RocksCommunitySync function below
type CommunityNotice struct {
	Community string `json:"community"`
	Action    string `json:"action"`
	User      Agent  `json:"user"`
}

// CommunityResponse is returned from a query request
type CommunityResponse struct {
	Community  string   `json:"community"`
	Title      string   `json:"title"`
	Members    []string `json:"members"`    // googleID
	Moderators []string `json:"moderators"` // googleID
	User       Agent    `json:"user"`       // (Members,Moderators || User) present, not both
}

// Agent is the data sent by enl.rocks -- the version sent in the CommunityResponse is different, but close enough for our purposes
type Agent struct {
	Gid      string `json:"gid"`
	TGId     int64  `json:"tgid"`
	Agent    string `json:"agentid"`
	Verified bool   `json:"verified"`
	Smurf    bool   `json:"smurf"`
	Fullname string `json:"name"`
}

// sent by rocks on community pushes
type rocksPushResponse struct {
	Error   string `json:"error"`
	Success bool   `json:"success"`
}

// Config contains configuration for interacting with the enl.rocks APIs.
var Config struct {
	// APIKey is the API Key for enl.rocks.
	APIKey string
	// CommunityEndpoint is the API endpoint for viewing community membership
	CommunityEndpoint string
	// StatusEndpoint is the API endpoint for getting user status
	StatusEndpoint string
	limiter        *rate.Limiter
}

// Callbacks constains methods from model needed here
var Callbacks struct {
	FromDB              func(GoogleID) (Agent, time.Time, error)
	ToDB                func(Agent) error
	AddAgentToTeam      func(gid, communityID string) error
	RemoveAgentFromTeam func(gid, communityID string) error
}

func init() {
	Config.CommunityEndpoint = "https://enlightened.rocks/comm/api/membership"
	Config.StatusEndpoint = "https://enlightened.rocks/api/user/status"
	Config.limiter = rate.NewLimiter(rate.Limit(0.5), 60)
}

// Start is called from main() to initialize the config
func Start(apikey string) {
	log.Debugw("startup", "enl.rocks API Key", apikey)
	Config.APIKey = apikey
}

func Active() bool {
	return !(Config.APIKey == "")
}

// Search checks a agent at enl.rocks and returns an Agent
func Search(id string) (Agent, error) {
	var agent Agent
	if Config.APIKey == "" {
		return agent, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), (3 * time.Second))
	defer cancel()
	if err := Config.limiter.Wait(ctx); err != nil {
		log.Warn(err)
		// just keep going
	}

	apiurl := fmt.Sprintf("%s/%s?apikey=%s", Config.StatusEndpoint, id, Config.APIKey)
	req, err := http.NewRequest("GET", apiurl, nil)
	if err != nil {
		// do not leak API key to logs
		err := fmt.Errorf("error establishing .rocks request")
		log.Errorw(err.Error(), "search", id)
		return agent, err
	}
	client := &http.Client{
		Timeout: (3 * time.Second),
	}
	resp, err := client.Do(req)
	if err != nil {
		// do not leak API key to logs
		err := fmt.Errorf("error executing .rocks request")
		log.Errorw(err.Error(), "search", id)
		return agent, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return agent, err
	}

	err = json.Unmarshal(body, &agent)
	if err != nil {
		log.Error(err)
		return agent, err
	}
	return agent, nil
}

// CommunitySync is called from the https server when it receives a push notification
func CommunitySync(msg json.RawMessage) error {
	// check the source? is the community key enough for this? I don't think so
	var rc CommunityNotice
	err := json.Unmarshal(msg, &rc)
	if err != nil {
		log.Error(err)
		return err
	}

	if rc.Action == "onJoin" {
		err := Callbacks.AddAgentToTeam(rc.User.Gid, rc.Community)
		if err != nil {
			log.Error(err)
			return err
		}
	} else {
		err := Callbacks.RemoveAgentFromTeam(rc.User.Gid, rc.Community)
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return nil
}

// CommunityMemberPull grabs the member list from the associated community at enl.rocks and adds each agent to the team
func CommunityMemberPull(communityID string) error {
	if communityID == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), (3 * time.Second))
	defer cancel()
	if err := Config.limiter.Wait(ctx); err != nil {
		log.Warn(err)
		// just keep going
	}

	apiurl := fmt.Sprintf("%s?key=%s", Config.CommunityEndpoint, communityID)
	req, err := http.NewRequest("GET", apiurl, nil)
	if err != nil {
		err := fmt.Errorf("error establishing community pull request")
		log.Error(err)
		return err
	}
	client := &http.Client{
		Timeout: (3 * time.Second),
	}
	resp, err := client.Do(req)
	if err != nil {
		err := fmt.Errorf("error executing community pull request")
		log.Error(err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return err
	}

	var rr CommunityResponse
	err = json.Unmarshal(body, &rr)
	if err != nil {
		log.Error(err)
		return err
	}

	for _, gid := range rr.Members {
		if err := Callbacks.AddAgentToTeam(gid, communityID); err != nil {
			log.Info(err)
			continue
		}
	}
	return nil
}

func AddToRemoteRocksCommunity(gid, communityID string) error {
	if communityID == "" || gid == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), (3 * time.Second))
	defer cancel()
	if err := Config.limiter.Wait(ctx); err != nil {
		log.Infow("timeout waiting on .rocks rate limiter", "GID", gid)
		// just keep going
	}

	// XXX use NewRequest/client
	apiurl := fmt.Sprintf("%s/%s?key=%s", Config.CommunityEndpoint, gid, communityID)
	// #nosec
	resp, err := http.PostForm(apiurl, url.Values{"Agent": {gid}})
	if err != nil {
		// default err leaks API key to logs
		err := fmt.Errorf("error adding agent to .rocks community")
		log.Errorw(err.Error(), "GID", gid)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return err
	}

	var rr rocksPushResponse
	err = json.Unmarshal(body, &rr)
	if err != nil {
		log.Error(err)
		log.Debug(string(body))
	}
	if !rr.Success {
		log.Error(rr.Error)
	}
	return nil
}

// RemoveFromRemoteRocksCommunity removes an agent from a Rocks Community IF that community has API enabled.
func RemoveFromRemoteRocksCommunity(gid, communityID string) error {
	if communityID == "" || gid == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Config.limiter.Wait(ctx); err != nil {
		log.Info(err)
		// just keep going
	}

	apiurl := fmt.Sprintf("%s/%s?key=%s", Config.CommunityEndpoint, gid, communityID)
	req, err := http.NewRequest("DELETE", apiurl, nil)
	if err != nil {
		log.Error(err)
		return err
	}
	client := &http.Client{
		Timeout: (3 * time.Second),
	}
	resp, err := client.Do(req)
	if err != nil {
		// default err leaks API key to logs
		err := fmt.Errorf("error removing agent from .rocks community")
		log.Errorw(err.Error(), "GID", gid)
		return err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return err
	}

	var rr rocksPushResponse
	err = json.Unmarshal(body, &rr)
	if err != nil {
		log.Error(err)
		return err
	}
	if !rr.Success {
		err = fmt.Errorf(rr.Error)
		log.Error(err)
		return err
	}
	return nil
}

func Authorize(gid GoogleID) bool {
	var a Agent

	fromdb, fetched, err := Callbacks.FromDB(gid)
	if err != nil {
		log.Error(err)
		return true
	}
	if fromdb.Agent == "" || fetched.Before(time.Now().Add(0-time.Hour)) {
		result, err := Search(string(gid))
		if err != nil {
			log.Error(err)
			return true
		}
		err = Callbacks.ToDB(result)
		if err != nil {
			log.Error(err)
		}
		a = result
	} else {
		a = fromdb
	}

	if a.Agent != "" && a.Smurf {
		log.Warnw("access denied", "GID", gid, "reason", "listed as smurf at enl.rocks")
		return false
	}
	return true
}
