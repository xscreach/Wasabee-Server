package PhDevHTTP

import (
	//	"errors"
	"encoding/json"
	"fmt"
	"net/http"
	//	"net/http/httputil"
	//	"strings"
	"io/ioutil"

	"golang.org/x/oauth2"
	//	"golang.org/x/oauth2/google"

	"github.com/cloudkucooland/PhDevBin"
	"github.com/gorilla/mux"
)

func setupRoutes(r *mux.Router) {
	// Upload function
	r.Methods("OPTIONS").HandlerFunc(optionsRoute)
	r.HandleFunc("/", uploadRoute).Methods("POST")

	// Static aliased HTML files
	r.HandleFunc("/", advancedStaticRoute(config.FrontendPath, "/index.html", routeOptions{
		ignoreExceptions: true,
		modifySource: func(body *string) {
			replaceBlockVariable(body, "if_fork", false)
		},
	})).Methods("GET")

	// Static files
	PhDevBin.Log.Notice("Including static files from: %s", config.FrontendPath)
	addStaticDirectory(config.FrontendPath, "/", r)
	addStaticDirectory(config.FrontendPath, "/static", r)

	// Oauth2 stuff
	r.HandleFunc("/login", googleRoute).Methods("GET")
	r.HandleFunc("/callback", callbackRoute).Methods("GET")

	// Documents
	r.HandleFunc("/draw", uploadRoute).Methods("POST")
	r.HandleFunc("/draw/{document}", setAuthTagRoute).Methods("POST").Queries("authtag", "{authtag}")
	r.HandleFunc("/draw/{document}", setAuthTagRoute).Methods("GET").Queries("authtag", "{authtag}")
	r.HandleFunc("/draw/{document}", getRoute).Methods("GET")
	r.HandleFunc("/draw/{document}", deleteRoute).Methods("DELETE")
	r.HandleFunc("/draw/{document}", updateRoute).Methods("PUT")
	// user info
	r.HandleFunc("/me", meSetIngressNameRoute).Methods("GET").Queries("name", "{name}")                // set my display name /me?name=deviousness
	r.HandleFunc("/me", meSetUserLocationRoute).Methods("GET").Queries("lat", "{lat}", "lon", "{lon}") // set my display name /me?name=deviousness
	r.HandleFunc("/me", meShowRoute).Methods("GET")                                                    // show my stats (agen name/tags)
	r.HandleFunc("/me/{tag}", meToggleTagRoute).Methods("GET").Queries("state", "{state}")             // /me/wonky-tag-1234?state={Off|On}
	r.HandleFunc("/me/{tag}", meRemoveTagRoute).Methods("DELETE")                                      // remove me from tag
	// tags
	r.HandleFunc("/tag/new", newTagRoute).Methods("POST", "GET").Queries("name", "{name}") // return the location of every user if authorized
	r.HandleFunc("/tag/{tag}", addUserToTagRoute).Methods("GET").Queries("key", "{key}")   // invite user to tag
	r.HandleFunc("/tag/{tag}", getTagRoute).Methods("GET")                                 // return the location of every user if authorized
	r.HandleFunc("/tag/{tag}", deleteTagRoute).Methods("DELETE")                           // remove the tag completely
	r.HandleFunc("/tag/{tag}/delete", deleteTagRoute).Methods("GET")                       // remove the tag completely
	r.HandleFunc("/tag/{tag}/edit", editTagRoute).Methods("GET")                           // GUI to do basic edit
	r.HandleFunc("/tag/{tag}/{key}", addUserToTagRoute).Methods("GET")                     // invite user to tag
	// r.HandleFunc("/tag/{tag}/{key}", addUserToTagRoute).Methods("GET").Queries("color", "{color}") // set agent color on this tag
	r.HandleFunc("/tag/{tag}/{key}/delete", delUserFmTagRoute).Methods("GET") // remove user from tag
	r.HandleFunc("/tag/{tag}/{key}", delUserFmTagRoute).Methods("DELETE")     // remove user from tag

	r.HandleFunc("/{document}", getRoute).Methods("GET")
	r.HandleFunc("/{document}", deleteRoute).Methods("DELETE")
	r.HandleFunc("/{document}", updateRoute).Methods("PUT")

	// 404 error page
	r.PathPrefix("/").HandlerFunc(notFoundRoute)
}

func optionsRoute(res http.ResponseWriter, req *http.Request) {
	// I think this is now taken care of in the middleware
	res.Header().Add("Allow", "GET, PUT, POST, OPTIONS, HEAD, DELETE")
	res.WriteHeader(200)
	return
}

func getRoute(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["document"]

	doc, err := PhDevBin.Request(id)
	if err != nil {
		notFoundRoute(res, req)
	}

	res.Header().Add("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(res, "%s", doc.Content)
}

func deleteRoute(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["document"]
	me, err := GetUserID(req)
	if me == "" {
		PhDevBin.Log.Error("Not logged in, cannot delete document")
		http.Error(res, "Unauthorized", http.StatusUnauthorized)
		return
	}

	doc, err := PhDevBin.Request(id)
	if err != nil {
		PhDevBin.Log.Error(err)
	}
	if me != doc.Uploader {
		PhDevBin.Log.Error("Attempt to delete document owned by someone else")
		http.Error(res, "Unauthorized", http.StatusUnauthorized)
		return
	}

	err = PhDevBin.Delete(id)
	if err != nil {
		PhDevBin.Log.Error(err)
	}

	res.Header().Add("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(res, "OK: document removed.\n")
}

func setAuthTagRoute(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	id := vars["document"]
	authtag := vars["authtag"]
	me, err := GetUserID(req)
	if me == "" {
		PhDevBin.Log.Error("Not logged in, cannot set authtag")
		http.Error(res, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// I don't like pushing authentication/authorization out to the main module, but...
	err = PhDevBin.SetAuthTag(id, authtag, me)
	if err != nil {
		PhDevBin.Log.Error(err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	res.Header().Add("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(res, "OK: document authtag set.\n")
}

func internalErrorRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", "text/plain; charset=utf-8")
	res.WriteHeader(500)
	fmt.Fprint(res, "Oh no, the server is broken! ಠ_ಠ\nYou should try again in a few minutes, there's probably a desperate admin running around somewhere already trying to fix it.\n")
}

func notFoundRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Cache-Control", "no-cache")
	res.Header().Add("Content-Type", "text/plain; charset=utf-8")
	res.WriteHeader(404)
	fmt.Fprint(res, "404: Maybe the document is expired or has been removed.\n")
}

func googleRoute(res http.ResponseWriter, req *http.Request) {
	url := config.googleOauthConfig.AuthCodeURL(config.oauthStateString)
	res.Header().Add("Cache-Control", "no-cache")
	http.Redirect(res, req, url, http.StatusTemporaryRedirect)
}

func callbackRoute(res http.ResponseWriter, req *http.Request) {
	type PhDevUser struct {
		Id    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	content, err := getUserInfo(req.FormValue("state"), req.FormValue("code"))
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	var m PhDevUser
	err = json.Unmarshal(content, &m)
	if err != nil {
		PhDevBin.Log.Notice(err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	ses, err := config.store.Get(req, config.sessionName)
	if err != nil {
		PhDevBin.Log.Notice(err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
	}

	ses.Values["id"] = m.Id
	ses.Values["name"] = m.Name
	ses.Save(req, res)

	err = PhDevBin.InsertOrUpdateUser(m.Id, m.Name)
	if err != nil {
		PhDevBin.Log.Notice(err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
	}
	http.Redirect(res, req, "/me?postauth=1", http.StatusPermanentRedirect)
}

func getUserInfo(state string, code string) ([]byte, error) {
	if state != config.oauthStateString {
		return nil, fmt.Errorf("invalid oauth state")
	}
	token, err := config.googleOauthConfig.Exchange(oauth2.NoContext, code)
	if err != nil {
		return nil, fmt.Errorf("code exchange failed: %s", err.Error())
	}
	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed getting user info: %s", err.Error())
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading response body: %s", err.Error())
	}
	return contents, nil
}

func GetUserID(req *http.Request) (string, error) {
	ses, err := config.store.Get(req, config.sessionName)
	if err != nil {
		return "", err
	}

	if ses.Values["id"] == nil {
		// PhDevBin.Log.Notice("GetUserID called for unauthenticated user")
		return "", nil
	}

	userID := ses.Values["id"].(string)
	return userID, nil
}
