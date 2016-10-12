package github

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http/httputil"
	"os"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/go-github/github"
)

const (
	AcceptHeader = "application/vnd.github.machine-man-preview+json"
	UserAgent    = "GopherCI"
)

type GitHub struct {
	id      string // id is the integration id
	keyFile string // keyFile is the path to private key

}

// New returns a GitHub object for use with GitHub integrations
// https://developer.github.com/changes/2016-09-14-Integrations-Early-Access/
// id is the integration identifier (such as 394), keyFile is the path to the
// private key provided to you by GitHub during the integration registration.
func New(id, keyFile string) (*GitHub, error) {
	g := &GitHub{
		id:      id,
		keyFile: keyFile,
	}
	return g, g.getToken()
}

// getToken authentica
func (g *GitHub) getToken() error {
	claims := &jwt.StandardClaims{
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(3 * time.Minute).Unix(),
		Issuer:    g.id,
	}

	bearer := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	key, err := ioutil.ReadFile(g.keyFile)
	if err != nil {
		return fmt.Errorf("getToken: could not read private key: %v", err)
	}

	block, _ := pem.Decode(key)
	if block == nil {
		return fmt.Errorf("getToken: could not decode pem private key: %v", err)
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("getToken: could not parse private key: %v", err)
	}

	ss, err := bearer.SignedString(privateKey)
	if err != nil {
		return fmt.Errorf("getToken: could not sign jwt: %s", err)
	}

	c := github.NewClient(nil)
	c.UserAgent = UserAgent

	req, err := c.NewRequest("POST", fmt.Sprintf("/installations/%v/access_tokens", "1722"), nil)
	if err != nil {
		return fmt.Errorf("getToken: could not create request: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", ss))
	req.Header.Set("Accept", AcceptHeader)

	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		log.Println("could not dump request out:", err)
	}
	log.Printf("%s", dump)

	resp, err := c.Do(req, os.Stdout)
	if err != nil {
		return fmt.Errorf("getToken: could not get access_tokens: %v", err)
	}

	dump, err = httputil.DumpResponse(resp.Response, false)
	if err != nil {
		log.Println("could not dump response:", err)
	}
	log.Printf("%s", dump)

	return nil
}
