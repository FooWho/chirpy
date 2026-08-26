package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/FooWho/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
}

type apiUser struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type apiChirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserId    uuid.UUID `json:"user_id"`
}

func (apc *apiConfig) GetHits() int {
	return int(apc.fileserverHits.Load())
}

func (apc *apiConfig) IncrementHits() {
	apc.fileserverHits.Add(int32(1))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.IncrementHits()
		next.ServeHTTP(w, r)
	})
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to obtain database\n")
	}
	dbQueries := database.New(db)
	myCfg := &apiConfig{fileserverHits: atomic.Int32{}, dbQueries: dbQueries, platform: platform}

	mux := http.NewServeMux()
	mux.Handle("/app/",
		myCfg.middlewareMetricsInc(
			http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("POST /admin/reset", myCfg.doReset)
	mux.HandleFunc("POST /api/chirps", myCfg.chirp)
	mux.HandleFunc("POST /api/users", myCfg.createUser)
	mux.HandleFunc("GET /admin/metrics", myCfg.getMetrics)

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

func replaceBadWords(s string) string {
	words := strings.Split(s, " ")
	for i, word := range words {
		if strings.ToLower(word) == "kerfuffle" ||
			strings.ToLower(word) == "sharbert" ||
			strings.ToLower(word) == "fornax" {
			words[i] = "****"
		}
	}
	cleanS := strings.Join(words, " ")
	return cleanS
}

func (cfg *apiConfig) chirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body   string    `json:"body"`
		UserId uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, 500, "Error decoding parameters")
		return
	}
	if len(params.Body) <= 140 {
		cleanedBody := replaceBadWords(params.Body)
		createChirpParams := database.CreateChirpParams{Body: cleanedBody, UserID: params.UserId}
		dbChirp, err := cfg.dbQueries.CreateChirp(r.Context(), createChirpParams)
		if err != nil {
			log.Printf("Error creating chirp: %s", err)
			respondWithError(w, 500, "Error creating chirp")
			return
		}
		validChirp := apiChirp{ID: dbChirp.ID, CreatedAt: dbChirp.CreatedAt, UpdatedAt: dbChirp.UpdatedAt, Body: dbChirp.Body, UserId: dbChirp.UserID}
		respondWithJSON(w, 201, validChirp)
	} else {
		respondWithError(w, 400, "Chirp too long")
	}
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	user := apiUser{}
	err := decoder.Decode(&user)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, 500, user.Email)
		return
	}
	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), user.Email)
	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithError(w, 500, user.Email)
		return
	}
	resp := apiUser{ID: dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}
	respondWithJSON(w, 201, resp)
}

func (cfg *apiConfig) getMetrics(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Content-Type", "text/html")
	w.WriteHeader(200)
	hits := cfg.GetHits()
	hitString := "<html>\n"
	hitString += "    <body>\n"
	hitString += "        <h1>Welcome, Chirpy Admin!</h1>\n"
	hitString += fmt.Sprintf("        <p>Chirpy has been visited %d times!</p>\n", hits)
	hitString += "    </body>\n"
	hitString += "</html>\n"
	w.Write([]byte(hitString))
}

func (cfg *apiConfig) doReset(w http.ResponseWriter, r *http.Request) {

	if cfg.platform == "dev" {
		cfg.fileserverHits = atomic.Int32{}
		err := cfg.dbQueries.ResetUsers(r.Context())
		if err != nil {
			respondWithError(w, 500, "Error resetting users table")
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	} else {
		respondWithError(w, 403, "Forbidden")
	}
}
