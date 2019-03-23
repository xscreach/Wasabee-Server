package PhDevHTTP

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cloudkucooland/PhDevBin"
)

// or use the PhDevBin.Location struct
// this is minimal for what we need here
type loc struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Type string  `json:"_type"`
}

func ownTracksRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json")

	gid, auth := ownTracksAuthentication(res, req)
	if auth == false {
		http.Error(res, "Error verifing authentication", http.StatusUnauthorized)
		return
	}

	contentType := strings.Split(strings.Replace(strings.ToLower(req.Header.Get("Content-Type")), " ", "", -1), ";")[0]
	if contentType != "application/json" {
		http.Error(res, "Invalid request (needs to be application/json)", http.StatusNotAcceptable)
		return
	}

	jBlob, err := ioutil.ReadAll(req.Body)
	if err != nil {
		PhDevBin.Log.Notice(err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if string(jBlob) == "" {
		PhDevBin.Log.Notice("empty JSON")
		fmt.Fprintf(res, "{ }")
		return
	}

	jRaw := json.RawMessage(jBlob)

	// PhDevBin.Log.Debug(string(jBlob))
	var t loc
	if err = json.Unmarshal(jBlob, &t); err != nil {
		PhDevBin.Log.Notice(err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	switch t.Type {
	case "location":
		PhDevBin.OwnTracksUpdate(gid, jRaw, t.Lat, t.Lon)
		s, err := PhDevBin.OwnTracksTeams(gid)
		if err != nil {
			PhDevBin.Log.Notice(err)
			http.Error(res, err.Error(), http.StatusInternalServerError)
		}
		fmt.Fprintf(res, string(s))
	case "transition":
		s, err := PhDevBin.OwnTracksTransition(gid, jRaw)
		if err != nil {
			PhDevBin.Log.Notice(err)
			http.Error(res, err.Error(), http.StatusInternalServerError)
		}
		fmt.Fprintf(res, string(s))
	case "waypoints":
		s, err := PhDevBin.OwnTracksSetWaypointList(gid, jRaw)
		if err != nil {
			PhDevBin.Log.Notice(err)
			// XXX use the cmd to send a URL to set primary team?
			// e := "{ \"err\": \"Is your primary team set?\" }" // XXX is there a JSON standard for this kind of message?
			// http.Error(res, e, http.StatusInternalServerError)
			// fmt.Fprintf(res, e)
			// return
		}
		fmt.Fprintf(res, string(s))
	case "waypoint":
		s, _ := PhDevBin.OwnTracksSetWaypoint(gid, jRaw)
		if err != nil {
			PhDevBin.Log.Notice(err)
			// e := "{ \"err\": \"Is your primary team set?\" }"
			// http.Error(res, e, http.StatusInternalServerError)
			// fmt.Fprintf(res, e)
			// return
		}
		fmt.Fprintf(res, string(s))
	default:
		PhDevBin.Log.Notice("unhandled type: " + t.Type)
		PhDevBin.Log.Debug(string(jRaw))
		fmt.Fprintf(res, "{ }")
	}
}

func ownTracksAuthentication(res http.ResponseWriter, req *http.Request) (string, bool) {
	lockey, otpw, ok := req.BasicAuth()
	if ok == false {
		PhDevBin.Log.Notice("Unable to decode basic authentication")
		return "", false
	}

	gid, err := PhDevBin.VerifyOwnTracksPW(lockey, otpw)
	if err != nil {
		PhDevBin.Log.Notice(err)
		return "", false
	}
	if gid == "" {
		PhDevBin.Log.Noticef("OwnTracks authentication failed for: %s", lockey)
		return "", false
	}

	return gid, true
}
