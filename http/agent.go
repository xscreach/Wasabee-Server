package wasabeehttps

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/wasabee-project/Wasabee-Server"
	"io/ioutil"
	"net/http"
	"strings"
)

func agentProfileRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	var agent wasabee.Agent

	// must be authenticated
	gid, err := getAgentID(req)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	togid, err := wasabee.ToGid(id)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	err = wasabee.FetchAgent(togid, &agent)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	agent.CanSendTo = gid.CanSendTo(togid)

	data, _ := json.Marshal(agent)
	fmt.Fprint(res, string(data))
}

func agentMessageRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	gid, err := getAgentID(req)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	togid, err := wasabee.ToGid(id)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	message := req.FormValue("m")
	if message == "" {
		message = "This is a toast notification"
	}

	ok := gid.CanSendTo(togid)
	if !ok {
		err := fmt.Errorf("forbidden: only team owners can send to agents on the team")
		wasabee.Log.Warnw(err.Error(), "GID", gid, "resource", togid)
		http.Error(res, jsonError(err), http.StatusForbidden)
		return
	}
	ok, err = togid.SendMessage(message)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	if !ok {
		err := fmt.Errorf("message did not send")
		wasabee.Log.Warnw(err.Error(), "from", gid, "to", togid, "contents", message)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(res, jsonStatusOK)
}

func agentTargetRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	gid, err := getAgentID(req)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	if contentTypeIs(req, "multipart/form-data") {
		wasabee.Log.Infow("using old format for sending targets", "GID", gid)
		agentTargetRouteOld(res, req)
		return
	}

	if !contentTypeIs(req, jsonTypeShort) {
		err := fmt.Errorf("must use content-type: %s", jsonTypeShort)
		wasabee.Log.Errorw(err.Error(), "GID", gid)
		http.Error(res, jsonError(err), http.StatusNotAcceptable)
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	togid, err := wasabee.ToGid(id)
	if err != nil {
		wasabee.Log.Error(err.Error())
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	jBlob, err := ioutil.ReadAll(req.Body)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	if string(jBlob) == "" {
		wasabee.Log.Warnw("empty JSON", "GID", gid)
		http.Error(res, jsonStatusEmpty, http.StatusNotAcceptable)
		return
	}

	jRaw := json.RawMessage(jBlob)

	type T struct {
		Name string
		Lat  string
		Lng  string
		ll   string
	}
	var target T
	err = json.Unmarshal(jRaw, &target)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	if target.Name == "" {
		err := fmt.Errorf("portal not set")
		wasabee.Log.Warnw(err.Error(), "GID", gid)
		http.Error(res, jsonError(err), http.StatusNotAcceptable)
		return
	}

	if target.Lat == "" || target.Lng == "" {
		err := fmt.Errorf("lat/ng not set")
		wasabee.Log.Warnw(err.Error(), "GID", gid)
		http.Error(res, jsonError(err), http.StatusNotAcceptable)
		return
	}

	iname, err := gid.IngressName()
	if err != nil {
		wasabee.Log.Error(err)
	}

	templateData := struct {
		Name   string
		Lat    string
		Lon    string
		Type   string
		Sender string
	}{
		Name:   target.Name,
		Lat:    target.Lat,
		Lon:    target.Lng,
		Type:   "ad-hoc target",
		Sender: iname,
	}

	msg, err := gid.ExecuteTemplate("target", templateData)
	if err != nil {
		wasabee.Log.Error(err)
		msg = fmt.Sprintf("template failed; ad-hoc target @ %s %s", target.Lat, target.Lng)
		// do not report send errors up the chain, just log
	}

	/* ok := gid.CanSendTo(togid)
	if !ok {
		err := fmt.Errorf("forbidden")
		wasabee.Log.Warnw(err.Error(), "from", gid, "to", togid,)
		http.Error(res, jsonError(err), http.StatusForbidden)
		return
	} */
	ok, err := togid.SendMessage(msg)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	if !ok {
		err := fmt.Errorf("message did not send")
		wasabee.Log.Warnw(err.Error(), "from", gid, "to", togid)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(res, jsonStatusOK)
}

func agentTargetRouteOld(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	gid, err := getAgentID(req)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	togid, err := wasabee.ToGid(id)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	portal := req.FormValue("portal")
	if portal == "" {
		err := fmt.Errorf("portal net set")
		wasabee.Log.Warnw(err.Error(), "GID", gid)
		http.Error(res, jsonError(err), http.StatusNotAcceptable)
		return
	}

	ll := req.FormValue("ll")
	if ll == "" {
		err := fmt.Errorf("ll not set")
		wasabee.Log.Warnw(err.Error(), "GID", gid)
		http.Error(res, jsonError(err), http.StatusNotAcceptable)
		return
	}

	lls := strings.Split(ll, ",")
	lls = lls[:2] // make it be exactly 2 long

	iname, err := gid.IngressName()
	if err != nil {
		wasabee.Log.Error(err)
	}

	templateData := struct {
		Name   string
		Lat    string
		Lon    string
		Type   string
		Sender string
	}{
		Name:   portal,
		Lat:    lls[0],
		Lon:    lls[1],
		Type:   "ad-hoc target",
		Sender: iname,
	}

	msg, err := gid.ExecuteTemplate("target", templateData)
	if err != nil {
		wasabee.Log.Error(err)
		msg = fmt.Sprintf("template failed: ad-hoc target @ %s", ll)
		// do not report send errors up the chain, just log
	}

	ok, err := togid.SendMessage(msg)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	if !ok {
		err := fmt.Errorf("message did not send")
		wasabee.Log.Warnw(err.Error(), "from", gid, "to", togid)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(res, jsonStatusOK)
}

func agentPictureRoute(res http.ResponseWriter, req *http.Request) {
	_, err := getAgentID(req)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	togid, err := wasabee.ToGid(id)
	if err != nil {
		wasabee.Log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	url := togid.GetPicture()
	http.Redirect(res, req, url, http.StatusPermanentRedirect)
}
