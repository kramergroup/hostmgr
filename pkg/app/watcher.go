package app

import (
	"fmt"
	"os"

	"github.com/apsdehal/go-logger"
	"github.com/gomodule/redigo/redis"
)

var log, _ = logger.New("watcher", 1, os.Stdout)

/*
RedisWatcher watches a Redis server for state changes and
invokes a callback function on key/value changes
*/
type RedisWatcher struct {
	URL    string       // redis://localhost:6379
	update func() error // callback function for updates
	filter string       // key-base filter
	conn   redis.Conn   // redis connection object
}

/*
NewWatcher creates a new RedisWatcher

@param redisURL The URL of the redis server
@param updateCallback a callback function that is invocked i
											f there is a change to the state
*/
func NewWatcher(redisURL string, baseFilter string, updateCallback func() error) *RedisWatcher {
	return &RedisWatcher{
		URL:    redisURL,
		update: updateCallback,
		filter: baseFilter,
		conn:   nil,
	}
}

/*
Connect establishes a new connection to the Redis server
*/
func (w *RedisWatcher) Connect() error {
	conn, err := redis.DialURL(w.URL)
	if err == nil {
		log.DebugF("Connected to %s", w.URL)
		w.conn = conn
	}
	return err
}

/*
isConnected returns true if a connection to the Redis server is establised
*/
func (w *RedisWatcher) isConnected() bool {
	return (w.conn != nil)
}

/*
Start begins listening for updates
*/
func (w *RedisWatcher) Start() error {
	if !w.isConnected() {
		err := w.Connect()
		if err != nil {
			log.DebugF("Error connecting to %s [%s]", w.URL, err.Error())
			return err
		}
	}

	/*
		Listen to all events of the key-space with key base
	*/
	log.Debug("Configuring Redis to listen for all keyspace events")
	w.conn.Do("CONFIG", "SET", "notify-keyspace-events", "KEA")

	psc := redis.PubSubConn{Conn: w.conn}
	psc.PSubscribe(fmt.Sprintf("__key*__:%s*", w.filter))
	for {
		switch msg := psc.Receive().(type) {
		case redis.Message:
			//log.DebugF("Message: %s %s\n", msg.Channel, msg.Data)
			err := w.update()
			if err != nil {
				log.Error(err.Error())
			}
		case redis.Subscription:
			//log.DebugF("Subscription: %s %s %d\n", msg.Kind, msg.Channel, msg.Count)
			if msg.Count == 0 {
				return nil
			}
		case error:
			return msg
		}
	}
}

/*
Stop ends listening for updates
*/
func (w *RedisWatcher) Stop() error {
	if w.isConnected() {
		err := w.conn.Close()
		if err == nil {
			w.conn = nil
		}
		return err
	}

	return nil
}
