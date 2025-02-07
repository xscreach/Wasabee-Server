package model

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/wasabee-project/Wasabee-Server/log"
)

// DefensiveKeyList is the list of all defensive keys
type DefensiveKeyList struct {
	DefensiveKeys []DefensiveKey
	Fetched       string
}

// DefensiveKey is a sub-struct of DefensiveKeyList
type DefensiveKey struct {
	GID      GoogleID `json:"GID"`
	PortalID PortalID `json:"PortalID"`
	CapID    string   `json:"CapID"`
	Count    int32    `json:"Count"`
	Name     string   `json:"Name"`
	Lat      string   `json:"Lat"`
	Lon      string   `json:"Lng"`
}

// ListDefensiveKeys gets all keys an agent is authorized to know about.
func (gid GoogleID) ListDefensiveKeys() (DefensiveKeyList, error) {
	var dkl DefensiveKeyList
	var name, lat, lon sql.NullString

	rows, err := db.Query("SELECT gid, portalID, capID, count, name, Y(loc) AS lat, X(loc) AS lon FROM defensivekeys WHERE gid IN (SELECT DISTINCT other.gid FROM agentteams=other, agentteams=me WHERE me.gid = ? AND me.loadWD = 1 AND other.teamID = me.teamID AND other.shareWD = 1)", gid)

	if err != nil && err != sql.ErrNoRows {
		log.Error(err)
		return dkl, err
	}
	defer rows.Close()

	for rows.Next() {
		dk := DefensiveKey{}
		err := rows.Scan(&dk.GID, &dk.PortalID, &dk.CapID, &dk.Count, &name, &lat, &lon)
		if err != nil {
			log.Error(err)
			continue
			// return dkl, err
		}
		if name.Valid {
			dk.Name = name.String
		} else {
			dk.Name = ""
		}
		if lat.Valid {
			dk.Lat = lat.String
		} else {
			dk.Lat = ""
		}
		if lon.Valid {
			dk.Lon = lon.String
		} else {
			dk.Lon = ""
		}
		dkl.DefensiveKeys = append(dkl.DefensiveKeys, dk)
	}

	dkl.Fetched = time.Now().Format(time.RFC3339)
	return dkl, nil
}

// InsertDefensiveKey adds a new key to the list
func (gid GoogleID) InsertDefensiveKey(dk DefensiveKey) error {
	if dk.Count < 1 {
		if _, err := db.Exec("DELETE FROM defensivekeys WHERE gid = ? AND portalID = ?", gid, dk.PortalID); err != nil {
			log.Error(err)
			return err
		}
	} else {
		// convert to float64 and back to reduce the garbage input
		var flat, flon float64

		flat, err := strconv.ParseFloat(dk.Lat, 64)
		if err != nil {
			log.Error(err)
			flat = float64(0)
		}

		flon, err = strconv.ParseFloat(dk.Lon, 64)
		if err != nil {
			log.Error(err)
			flon = float64(0)
		}
		point := fmt.Sprintf("POINT(%s %s)", strconv.FormatFloat(flon, 'f', 7, 64), strconv.FormatFloat(flat, 'f', 7, 64))

		if _, err := db.Exec("INSERT INTO defensivekeys (gid, portalID, capID, count, name, loc) VALUES (?, ?, ?, ?, ?, PointFromText(?)) ON DUPLICATE KEY UPDATE capID = ?, count = ?", gid, dk.PortalID, dk.CapID, dk.Count, dk.Name, point, dk.CapID, dk.Count); err != nil {
			log.Error(err)
			return err
		}
	}
	return nil
}
