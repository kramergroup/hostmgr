package app

import (
	"encoding/json"
	"fmt"

	"github.com/gomodule/redigo/redis"
)

/*
RedisQuery provides an interface for querying the backing Redis service
for relevant data
*/
type RedisQuery struct {
	URL    string
	filter string
}

/*
NewQuery creates a new Redis query
*/
func NewQuery(aURL, aFilter string) *RedisQuery {
	return &RedisQuery{URL: aURL, filter: aFilter}
}

/*
HostDefinitions queries redis for all host definitions
*/
func (w *RedisQuery) HostDefinitions() ([]HostDefinition, error) {
	c, err := redis.DialURL(w.URL)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	keys, errA := redis.Strings(c.Do("Keys", w.filter+"*"))
	if errA != nil {
		return nil, errA
	}
	log.Debug(fmt.Sprintf("Received %d host definition keys", len(keys)))
	var defs []HostDefinition
	for _, key := range keys {

		value, errB := redis.String(c.Do("GET", key))
		if errB != nil {
			err = errB // store last error and return later
		}
		var b HostDefinition
		errC := json.Unmarshal([]byte(value), &b)
		if errC == nil {
			defs = append(defs, b)
		} else {
			log.Debug(fmt.Sprintf("Error unmarshalling HostDefinition [%s]", errC.Error()))
			err = errC // store last error and return later
		}
		log.Debug(fmt.Sprintf("Unmarshalled %d HostDefinitions from query", len(defs)))
	}

	return defs, err
}

/*
Announce creates or updates a host definition record
*/
func (w *RedisQuery) Announce(def HostDefinition) (string, error) {

	key := w.generateKey(def)

	c, err := redis.DialURL(w.URL)
	if err != nil {
		return key, err
	}
	defer c.Close()

	v, err := json.Marshal(def)
	if err != nil {
		return key, err
	}

	_, err = c.Do("SET", key, v)
	return key, err
}

/*
Revoke removes a host definition record
*/
func (w *RedisQuery) Revoke(key string) error {

	c, err := redis.DialURL(w.URL)
	if err != nil {
		return err
	}
	defer c.Close()

	_, err = c.Do("DEL", key)
	return err
}

/*
generateKey generates a unique key for each HostDefinition
*/
func (w *RedisQuery) generateKey(def HostDefinition) string {
	return fmt.Sprintf("%s/hosts/%s-%s-%s", w.filter, def.Hostname, def.IP, def.ClientUser)
}
