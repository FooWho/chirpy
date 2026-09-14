package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/FooWho/chirpy/internal/auth"
	"github.com/FooWho/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type contextKey string

const userIDKey contextKey = "userID"

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	tokenSecret    string
}

type apiUser struct {
	ID             uuid.UUID `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"hashed_password"`
	Password       string    `json:"password"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	tokenSecret := os.Getenv("TOKEN_SECRET")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to obtain database\n")
	}
	dbQueries := database.New(db)
	myCfg := &apiConfig{fileserverHits: atomic.Int32{}, dbQueries: dbQueries, platform: platform, tokenSecret: tokenSecret}

	mux := http.NewServeMux()
	mux.Handle("/app/",
		myCfg.middlewareMetricsInc(
			http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	// Unprotected Endpoints
	mux.HandleFunc("GET /api/chirps", myCfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{id}", myCfg.getChirpById)
	mux.HandleFunc("GET /admin/metrics", myCfg.getMetrics)
	mux.HandleFunc("POST /api/users", myCfg.createUser)
	mux.HandleFunc("POST /api/login", myCfg.loginUser)
	mux.HandleFunc("POST /api/polka/webhooks", myCfg.setUserToRed)

	// Protected Endpoints
	mux.HandleFunc("PUT /api/users", myCfg.middlewareLoggedIn(myCfg.updateUser))
	mux.HandleFunc("POST /api/chirps", myCfg.middlewareLoggedIn(myCfg.createChirp))
	mux.HandleFunc("DELETE /api/chirps/{id}", myCfg.middlewareLoggedIn(myCfg.deleteChirp))

	// Special Endpoints
	mux.HandleFunc("POST /admin/reset", myCfg.doReset)
	mux.HandleFunc("POST /api/refresh", myCfg.refreshUser)
	mux.HandleFunc("POST /api/revoke", myCfg.revokeRefresh)

	s := &http.Server{Addr: ":8080", Handler: mux}
	s.ListenAndServe()
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type parameters struct {
		Error string `json:"error"`
	}

	params := parameters{}
	params.Error = msg

	dat, err := json.Marshal(params)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		respondWithError(w, 500, "Error marshalling json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		respondWithError(w, 500, "Error marshalling json")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func (cfg *apiConfig) GetSecret() ([]byte, error) {
	return []byte(cfg.tokenSecret), nil
}

func (cfg *apiConfig) middlewareLoggedIn(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		id, err := auth.AuthenticateUser(r.Header, cfg.tokenSecret)
		if err != nil {
			log.Print("Bad token for user")
			respondWithError(w, http.StatusUnauthorized, "Bad token for user")
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, id)
		reqWithContext := r.WithContext(ctx)

		next(w, reqWithContext)
	}
}
