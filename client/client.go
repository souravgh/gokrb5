// Package client provides a client library and methods for Kerberos 5 authentication.
package client

import (
	"errors"
	"fmt"

	"gopkg.in/jcmturner/gokrb5.v3/config"
	"gopkg.in/jcmturner/gokrb5.v3/credentials"
	"gopkg.in/jcmturner/gokrb5.v3/crypto"
	"gopkg.in/jcmturner/gokrb5.v3/crypto/etype"
	"gopkg.in/jcmturner/gokrb5.v3/iana/errorcode"
	"gopkg.in/jcmturner/gokrb5.v3/iana/nametype"
	"gopkg.in/jcmturner/gokrb5.v3/keytab"
	"gopkg.in/jcmturner/gokrb5.v3/messages"
	"gopkg.in/jcmturner/gokrb5.v3/types"
)

// Client side configuration and state.
type Client struct {
	Credentials *credentials.Credentials
	Config      *config.Config
	GoKrb5Conf  *Config
	sessions    *sessions
	Cache       *Cache
}

// Config struct holds GoKRB5 specific client configurations.
// Set Disable_PA_FX_FAST to true to force this behaviour off.
// Set Assume_PA_ENC_TIMESTAMP_Required to send the PA_ENC_TIMESTAMP pro-actively rather than waiting for a KRB_ERROR response from the KDC indicating it is required.
type Config struct {
	DisablePAFXFast              bool
	AssumePAEncTimestampRequired bool
}

// NewClientWithPassword creates a new client from a password credential.
// Set the realm to empty string to use the default realm from config.
func NewClientWithPassword(username, realm, password string) Client {
	creds := credentials.NewCredentials(username, realm)
	return Client{
		Credentials: creds.WithPassword(password),
		Config:      config.NewConfig(),
		GoKrb5Conf:  &Config{},
		sessions: &sessions{
			Entries: make(map[string]*session),
		},
		Cache: NewCache(),
	}
}

// NewClientWithKeytab creates a new client from a keytab credential.
func NewClientWithKeytab(username, realm string, kt keytab.Keytab) Client {
	creds := credentials.NewCredentials(username, realm)
	return Client{
		Credentials: creds.WithKeytab(kt),
		Config:      config.NewConfig(),
		GoKrb5Conf:  &Config{},
		sessions: &sessions{
			Entries: make(map[string]*session),
		},
		Cache: NewCache(),
	}
}

// NewClientFromCCache create a client from a populated client cache.
//
// WARNING: If you do not add a keytab or password to the client then the TGT cannot be renewed and a failure will occur after the TGT expires.
func NewClientFromCCache(c credentials.CCache) (Client, error) {
	cl := Client{
		Credentials: c.GetClientCredentials(),
		Config:      config.NewConfig(),
		GoKrb5Conf:  &Config{},
		sessions: &sessions{
			Entries: make(map[string]*session),
		},
		Cache: NewCache(),
	}
	spn := types.PrincipalName{
		NameType:   nametype.KRB_NT_SRV_INST,
		NameString: []string{"krbtgt", c.DefaultPrincipal.Realm},
	}
	cred, ok := c.GetEntry(spn)
	if !ok {
		return cl, errors.New("TGT not found in CCache")
	}
	var tgt messages.Ticket
	err := tgt.Unmarshal(cred.Ticket)
	if err != nil {
		return cl, fmt.Errorf("TGT bytes in cache are not valid: %v", err)
	}
	cl.sessions.Entries[c.DefaultPrincipal.Realm] = &session{
		Realm:      c.DefaultPrincipal.Realm,
		AuthTime:   cred.AuthTime,
		EndTime:    cred.EndTime,
		RenewTill:  cred.RenewTill,
		TGT:        tgt,
		SessionKey: cred.Key,
	}
	for _, cred := range c.GetEntries() {
		var tkt messages.Ticket
		err = tkt.Unmarshal(cred.Ticket)
		if err != nil {
			return cl, fmt.Errorf("Cache entry ticket bytes are not valid: %v", err)
		}
		cl.Cache.addEntry(
			tkt,
			cred.AuthTime,
			cred.StartTime,
			cred.EndTime,
			cred.RenewTill,
			cred.Key,
		)
	}
	return cl, nil
}

// WithConfig sets the Kerberos configuration for the client.
func (cl *Client) WithConfig(cfg *config.Config) *Client {
	cl.Config = cfg
	return cl
}

// WithKeytab adds a keytab to the client
func (cl *Client) WithKeytab(kt keytab.Keytab) *Client {
	cl.Credentials.WithKeytab(kt)
	return cl
}

// WithPassword adds a password to the client
func (cl *Client) WithPassword(password string) *Client {
	cl.Credentials.WithPassword(password)
	return cl
}

// Key returns a key for the client. Preferably from a keytab and then generated from the password.
// The KRBError would have been returned from the KDC and must be of type KDC_ERR_PREAUTH_REQUIRED.
// If a KRBError is not available pass nil and a key will be returned from the credentials keytab.
func (cl *Client) Key(etype etype.EType, krberr messages.KRBError) (types.EncryptionKey, error) {
	if cl.Credentials.HasKeytab() && etype != nil {
		return cl.Credentials.Keytab.GetEncryptionKey(cl.Credentials.CName.NameString, cl.Credentials.Realm, 0, etype.GetETypeID())
	} else if cl.Credentials.HasPassword() {
		if krberr.ErrorCode == errorcode.KDC_ERR_PREAUTH_REQUIRED {
			var pas types.PADataSequence
			err := pas.Unmarshal(krberr.EData)
			if err != nil {
				return types.EncryptionKey{}, fmt.Errorf("Could not get PAData from KRBError to generate key from password: %v", err)
			}
			key, _, err := crypto.GetKeyFromPassword(cl.Credentials.Password, krberr.CName, krberr.CRealm, etype.GetETypeID(), pas)
			return key, err
		}
		key, _, err := crypto.GetKeyFromPassword(cl.Credentials.Password, cl.Credentials.CName, cl.Credentials.Realm, etype.GetETypeID(), types.PADataSequence{})
		return key, err
	}
	return types.EncryptionKey{}, errors.New("Credential has neither keytab or password to generate key.")
}

// LoadConfig loads the Kerberos configuration for the client from file path specified.
func (cl *Client) LoadConfig(cfgPath string) (*Client, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return cl, err
	}
	cl.Config = cfg
	return cl, nil
}

// IsConfigured indicates if the client has the values required set.
func (cl *Client) IsConfigured() (bool, error) {
	// Client needs to have either a password, keytab or a session already (later when loading from CCache)
	if !cl.Credentials.HasPassword() && !cl.Credentials.HasKeytab() {
		sess, err := cl.GetSessionFromRealm(cl.Config.LibDefaults.DefaultRealm)
		if err != nil || sess.AuthTime.IsZero() {
			return false, errors.New("client has neither a keytab nor a password set and no session")
		}
	}
	if cl.Credentials.Username == "" {
		return false, errors.New("client does not have a username")
	}
	if cl.Config.LibDefaults.DefaultRealm == "" {
		return false, errors.New("client krb5 config does not have a default realm")
	}
	if !cl.Config.LibDefaults.DNSLookupKDC {
		for _, r := range cl.Config.Realms {
			if r.Realm == cl.Config.LibDefaults.DefaultRealm {
				if len(r.KDC) > 0 {
					return true, nil
				}
				return false, errors.New("client krb5 config does not have any defined KDCs for the default realm")
			}
		}
	}
	return true, nil
}

// Login the client with the KDC via an AS exchange.
func (cl *Client) Login() error {
	if cl.Credentials.Realm == "" {
		cl.Credentials.Realm = cl.Config.LibDefaults.DefaultRealm
	}
	return cl.ASExchange(cl.Credentials.Realm, 0)
}
