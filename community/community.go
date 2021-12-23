package community

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	"github.com/lestrrat-go/jwx/jwt"

	"github.com/wasabee-project/Wasabee-Server/config"
	"github.com/wasabee-project/Wasabee-Server/log"
	"github.com/wasabee-project/Wasabee-Server/model"
)

// the top-level data structure defined by the community website
type pull struct {
	Profile profile // the one we are concerrned with
	// the following are present on errors
	Code      uint16
	Exception string
	Class     string
	// all other fields are ignored
}

// the profile type defined by the community website
type profile struct {
	Name  string
	About string
	// all other fields are ignored
}

const profileURL = "https://community.ingress.com/en/profile"
const xgid = "x-gid"
const xme = "x-me"
const aud = "c2g"

// Validate checks the community website for the token and makes sure the token is correct
func Validate(gid model.GoogleID, name string) (bool, error) {
	profile, err := fetch(name)
	if err != nil {
		return false, err
	}

	if err := checkJWT(strings.TrimSpace(profile.About), name, gid); err != nil {
		return false, nil // nil to trigger NotAcceptable rather than InternalServerError
	}

	gid.SetCommunityName(name)
	log.Infow("validated niantic community name", "gid", gid, "name", name)
	return true, nil
}

func fetch(name string) (*profile, error) {
	p := pull{}

	apiurl := fmt.Sprintf("%s/%s.json", profileURL, name)
	req, err := http.NewRequest("GET", apiurl, nil)
	if err != nil {
		log.Errorw(err.Error(), "fetch", name)
		return &p.Profile, err
	}
	client := &http.Client{
		Timeout: (3 * time.Second),
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorw(err.Error(), "fetch", name)
		return &p.Profile, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return &p.Profile, err
	}

	if err = json.Unmarshal(body, &p); err != nil {
		log.Error(err)
		return &p.Profile, err
	}
	if p.Exception != "" {
		err := fmt.Errorf(p.Exception)
		log.Errorw(err.Error(), "code", p.Code, "class", p.Class)
		return &p.Profile, nil
	}

	return &p.Profile, nil
}

// move the constants into the config package
func checkJWT(raw, name string, gid model.GoogleID) error {
	token, err := jwt.Parse([]byte(raw), jwt.InferAlgorithmFromKey(true), jwt.UseDefaultKey(true), jwt.WithKeySet(config.Get().JWParsingKeys))
	if err != nil {
		log.Errorw("community token parse failed", "err", err.Error(), "gid", gid, "name", name)
		return err
	}

	if err := jwt.Validate(token, jwt.WithAudience(aud), jwt.WithClaimValue(xme, name), jwt.WithClaimValue(xgid, string(gid))); err != nil {
		log.Errorw("community token validate failed", "err", err.Error(), "gid", gid, "name", name)
		return err
	}
	return nil
}

// BuildToken generates a token to be posted on the community site to verify the agent's name
func BuildToken(gid model.GoogleID, name string) (string, error) {
	key, ok := config.Get().JWSigningKeys.Get(0)
	if !ok {
		err := fmt.Errorf("encryption jwk not set")
		log.Error(err)
		return "", err
	}

	jwts, err := jwt.NewBuilder().
		Claim(xgid, string(gid)).
		Claim(xme, name).
		Audience([]string{aud}).
		Build()
	if err != nil {
		log.Error(err)
		return "", err
	}

	hdrs := jws.NewHeaders()
	hdrs.Set("jku", config.JKU())

	signed, err := jwt.Sign(jwts, jwa.RS256, key, jwt.WithHeaders(hdrs))
	if err != nil {
		log.Error(err)
		return "", err
	}
	return string(signed), nil
}
