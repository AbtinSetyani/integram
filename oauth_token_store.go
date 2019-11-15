package integram

import (
	"fmt"
	"time"

	"gopkg.in/mgo.v2/bson"
)

type OAuthTokenStore interface {
	GetOAuthAccessToken(user *User) (token string, expireDate *time.Time, err error)
	SetOAuthAccessToken(user *User, token string, expireDate *time.Time) error
	GetOAuthRefreshToken(user *User) (string, error)
	SetOAuthRefreshToken(user *User, token string) error
}

type DefaultOAuthTokenMongoStore struct {
}

var oauthTokenStore OAuthTokenStore = &DefaultOAuthTokenMongoStore{}

func SetOAuthTokenStore(store OAuthTokenStore) {
	oauthTokenStore = store
}

func MigrateToDefault(c *Context, oldTS OAuthTokenStore) (total int, migrated int, expired int, err error) {
	users := []userData{}
	keyPrefix := "protected." + c.ServiceName
	err = c.db.C("users").Find(bson.M{keyPrefix + ".oauthvalid": true}).Select(bson.M{"_id": 1}).All(&users)
	if err != nil {
		return
	}

	total = len(users)
	for i, userData := range users {
		if i%100 == 0 {
			fmt.Printf("MigrateToDefault: %d/%d tokens transfered\n", i, len(users))
		}

		user := userData.User

		at, _, err := oldTS.GetOAuthAccessToken(&user)
		if err != nil {
			c.Log().Errorf("OAuthTokenStore MigrateToDefault got error on SetOAuthAccessToken: %s", err.Error())
			continue
		}

		rt, err := oldTS.GetOAuthRefreshToken(&user)
		if err != nil {
			c.Log().Errorf("OAuthTokenStore MigrateToDefault got error on SetOAuthRefreshToken: %s", err.Error())
			continue
		}

		err = c.db.C("users").UpdateId(user.ID, bson.M{"$set": bson.M{keyPrefix + ".oauthtoken": at, keyPrefix + ".oauthrefreshtoken": rt, keyPrefix + ".oauthvalid": false}})
		if err != nil {
			c.Log().Errorf("OAuthTokenStore MigrateToDefault got error: %s", err.Error())
			continue
		}

		migrated++
	}

	return
}

func MigrateFromDefault(c *Context, newTS OAuthTokenStore) (total int, migrated int, expired int, err error) {
	users := []userData{}
	keyPrefix := "protected." + c.ServiceName
	err = c.db.C("users").Find(bson.M{keyPrefix + ".oauthtoken": bson.M{"$exists": true, "$ne": ""}}).Select(bson.M{keyPrefix + ".oauthtoken": 1, keyPrefix + ".oauthexpiredate": 1, keyPrefix + ".oauthrefreshtoken": 1}).All(&users)
	if err != nil {
		return
	}

	total = len(users)
	now := time.Now()
	for i, userData := range users {
		if i%100 == 0 {
			fmt.Printf("MigrateFromDefault: %d/%d users transfered\n", i, len(users))
		}

		user := userData.User
		if ps, exists := userData.Protected[c.ServiceName]; exists {
			// skip expired tokens without refresh tokens
			if ps.OAuthExpireDate.Before(now) && ps.OAuthRefreshToken == "" {
				expired++
				continue
			}

			err = newTS.SetOAuthAccessToken(&user, ps.OAuthToken, ps.OAuthExpireDate)
			if err != nil {
				c.Log().Errorf("OAuthTokenStore MigrateFromDefault got error on SetOAuthAccessToken: %s", err.Error())
				continue
			}

			err = newTS.SetOAuthRefreshToken(&user, ps.OAuthRefreshToken)
			if err != nil {
				c.Log().Errorf("OAuthTokenStore MigrateFromDefault got error on SetOAuthRefreshToken: %s", err.Error())
				continue
			}

			err = c.db.C("users").UpdateId(user.ID, bson.M{"$set": bson.M{keyPrefix + ".oauthtoken": "", keyPrefix + ".oauthrefreshtoken": "", keyPrefix + ".oauthvalid": true}})
			if err != nil {
				c.Log().Errorf("OAuthTokenStore MigrateFromDefault got error: %s", err.Error())
				continue
			}

			migrated++
		}
	}

	return
}

func (d *DefaultOAuthTokenMongoStore) GetOAuthAccessToken(user *User) (token string, expireDate *time.Time, err error) {
	ps, err := user.protectedSettings()
	if err != nil {
		return "", nil, err
	}

	return ps.OAuthToken, nil, nil
}

func (d *DefaultOAuthTokenMongoStore) GetOAuthRefreshToken(user *User) (string, error) {
	ps, err := user.protectedSettings()

	if err != nil {
		return "", err
	}

	return ps.OAuthRefreshToken, nil
}

func (d *DefaultOAuthTokenMongoStore) SetOAuthAccessToken(user *User, token string, expireDate *time.Time) error {
	ps, err := user.protectedSettings()
	if err != nil {
		return err
	}

	ps.OAuthToken = token
	ps.OAuthExpireDate = expireDate

	return user.saveProtectedSettings()
}

func (d *DefaultOAuthTokenMongoStore) SetOAuthRefreshToken(user *User, refreshToken string) error {
	ps, err := user.protectedSettings()
	if err != nil {
		return err
	}

	ps.OAuthRefreshToken = refreshToken

	return user.saveProtectedSetting("OAuthRefreshToken", refreshToken)
}
