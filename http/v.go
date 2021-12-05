package wasabeehttps

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/wasabee-project/Wasabee-Server/log"
	"github.com/wasabee-project/Wasabee-Server/model"
	"github.com/wasabee-project/Wasabee-Server/v"
)

func vTeamRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)

	// the POST is empty, all we have is the teamID from the URL
	vars := mux.Vars(req)
	id := vars["teamID"]
	if id == "" {
		err := fmt.Errorf("V hook called with empty team ID")
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	log.Infow("V requested team sync", "server", req.RemoteAddr, "team", id)

	vteam, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	teams, err := model.GetTeamsByVID(vteam)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	keys := make(map[model.GoogleID]string)

	for _, teamID := range teams {
		gid, err := teamID.Owner()
		if err != nil {
			log.Error(err)
			continue
		}

		key := keys[gid]
		if key != "" {
			key, err = gid.VAPIkey()
			if err != nil {
				log.Error(err)
				continue
			}
			if key == "" {
				log.Errorw("no VAPI key for team owner, skipping sync", "GID", gid, "teamID", teamID, "vteam", vteam)
				continue
			}
			keys[gid] = key
		}

		err = v.Sync(teamID, key)
		if err != nil {
			log.Error(err)
			http.Error(res, jsonError(err), http.StatusInternalServerError)
			return
		}
	}

	fmt.Fprint(res, jsonStatusOK)
}

func vPullTeamRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	gid, err := getAgentID(req)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	team := model.TeamID(vars["team"])

	owns, err := gid.OwnsTeam(team)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	if !owns {
		err := fmt.Errorf("attempt to pull V for a team owned by someone else")
		log.Errorw(err.Error(), "GID", gid, "teamID", team)
		http.Error(res, jsonError(err), http.StatusForbidden)
		return
	}

	vkey, err := gid.VAPIkey()
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	if vkey == "" {
		err := fmt.Errorf("V API key not configured")
		log.Errorw(err.Error(), "GID", gid)
		http.Error(res, jsonError(err), http.StatusForbidden)
		return
	}

	if err := v.Sync(team, vkey); err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(res, jsonStatusOK)
}

func vConfigureTeamRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	gid, err := getAgentID(req)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	team := model.TeamID(vars["team"])

	owns, err := gid.OwnsTeam(team)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	if !owns {
		err := fmt.Errorf("attempt to configure V for a team owned by someone else")
		log.Errorw(err.Error(), "gid", gid, "teamID", team)
		http.Error(res, jsonError(err), http.StatusForbidden)
		return
	}

	vteam, err := strconv.ParseInt(req.FormValue("vteam"), 10, 64) // "0" to disable
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	r, err := strconv.ParseInt(req.FormValue("role"), 10, 8) // "0" for all roles
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	role := uint8(r)

	log.Infow("linking team to V", "GID", gid, "teamID", team, "vteam", vteam, "role", role)
	if err := team.VConfigure(vteam, role); err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(res, jsonStatusOK)
}

func vBulkImportRoute(res http.ResponseWriter, req *http.Request) {
	res.Header().Add("Content-Type", jsonType)
	gid, err := getAgentID(req)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(req)
	mode := vars["mode"]

	err = v.BulkImport(gid, mode)
	if err != nil {
		log.Error(err)
		http.Error(res, jsonError(err), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(res, jsonStatusOK)
}
