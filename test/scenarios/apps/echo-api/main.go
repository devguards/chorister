// echo-api: minimal HTTP server for scenario testing.
// Reads DATABASE_URL, REDIS_URL, NATS_URL from environment,
// provides health/status/connectivity endpoints.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

type BackendStatus struct {
	DB    string `json:"db"`
	Cache string `json:"cache"`
	Queue string `json:"queue"`
}

var (
	mu     sync.RWMutex
	status BackendStatus

	dbConn   *sql.DB
	rdClient *redis.Client
	natsConn *nats.Conn
	lastMsg  string
)

func connect() {
	// Database
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err != nil || db.PingContext(context.Background()) != nil {
			log.Printf("Database connection failed: %v", err)
			mu.Lock()
			status.DB = "error"
			mu.Unlock()
		} else {
			log.Println("Connected to database successfully")
			dbConn = db
			mu.Lock()
			status.DB = "ok"
			mu.Unlock()
		}
	} else {
		mu.Lock()
		status.DB = "not-configured"
		mu.Unlock()
	}

	// Redis/Cache
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Printf("Redis URL parse failed: %v", err)
			mu.Lock()
			status.Cache = "error"
			mu.Unlock()
		} else {
			rdb := redis.NewClient(opt)
			if err := rdb.Ping(context.Background()).Err(); err != nil {
				log.Printf("Cache connection failed: %v", err)
				mu.Lock()
				status.Cache = "error"
				mu.Unlock()
			} else {
				log.Println("Connected to cache successfully")
				rdClient = rdb
				mu.Lock()
				status.Cache = "ok"
				mu.Unlock()
			}
		}
	} else {
		mu.Lock()
		status.Cache = "not-configured"
		mu.Unlock()
	}

	// NATS
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		nc, err := nats.Connect(natsURL)
		if err != nil {
			log.Printf("NATS connection failed: %v", err)
			mu.Lock()
			status.Queue = "error"
			mu.Unlock()
		} else {
			log.Println("Connected to NATS successfully")
			natsConn = nc
			mu.Lock()
			status.Queue = "ok"
			mu.Unlock()
		}
	} else {
		mu.Lock()
		status.Queue = "not-configured"
		mu.Unlock()
	}
}

func healthz(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	s := status
	mu.RUnlock()

	healthy := true
	if os.Getenv("DATABASE_URL") != "" && s.DB != "ok" {
		healthy = false
	}
	if os.Getenv("REDIS_URL") != "" && s.Cache != "ok" {
		healthy = false
	}
	if os.Getenv("NATS_URL") != "" && s.Queue != "ok" {
		healthy = false
	}

	if healthy {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "degraded")
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	s := status
	mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func envHandler(w http.ResponseWriter, r *http.Request) {
	env := map[string]string{}
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		key := parts[0]
		// Skip secrets
		if strings.Contains(strings.ToLower(key), "password") ||
			strings.Contains(strings.ToLower(key), "secret") ||
			strings.Contains(strings.ToLower(key), "token") {
			continue
		}
		if len(parts) == 2 {
			env[key] = parts[1]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env)
}

func writeDB(w http.ResponseWriter, r *http.Request) {
	if dbConn == nil {
		http.Error(w, "database not connected", http.StatusServiceUnavailable)
		return
	}
	_, err := dbConn.ExecContext(r.Context(),
		"CREATE TABLE IF NOT EXISTS echo_rows (id SERIAL, ts TIMESTAMP DEFAULT NOW())")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = dbConn.ExecContext(r.Context(), "INSERT INTO echo_rows DEFAULT VALUES")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "ok")
}

func readDB(w http.ResponseWriter, r *http.Request) {
	if dbConn == nil {
		http.Error(w, "database not connected", http.StatusServiceUnavailable)
		return
	}
	var count int
	err := dbConn.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM echo_rows").Scan(&count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "rows: %d\n", count)
}

func publish(w http.ResponseWriter, r *http.Request) {
	if natsConn == nil {
		http.Error(w, "nats not connected", http.StatusServiceUnavailable)
		return
	}
	msg := fmt.Sprintf("hello from echo-api at %s", time.Now().Format(time.RFC3339))
	if err := natsConn.Publish("echo.messages", []byte(msg)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Published to NATS: %s", msg)
	fmt.Fprintln(w, "published")
}

func subscribe(w http.ResponseWriter, r *http.Request) {
	if natsConn == nil {
		http.Error(w, "nats not connected", http.StatusServiceUnavailable)
		return
	}
	sub, err := natsConn.SubscribeSync("echo.messages")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer sub.Unsubscribe()
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		http.Error(w, "no message: "+err.Error(), http.StatusRequestTimeout)
		return
	}
	log.Printf("Received from NATS: %s", string(msg.Data))
	fmt.Fprintf(w, "received: %s\n", string(msg.Data))
}

func cacheSet(w http.ResponseWriter, r *http.Request) {
	if rdClient == nil {
		http.Error(w, "cache not connected", http.StatusServiceUnavailable)
		return
	}
	key := "echo:last-set"
	val := time.Now().Format(time.RFC3339)
	if err := rdClient.Set(r.Context(), key, val, time.Hour).Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "ok")
}

func main() {
	go connect()

	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/env", envHandler)
	http.HandleFunc("/write-db", writeDB)
	http.HandleFunc("/read-db", readDB)
	http.HandleFunc("/publish", publish)
	http.HandleFunc("/subscribe", subscribe)
	http.HandleFunc("/cache-set", cacheSet)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("echo-api listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
