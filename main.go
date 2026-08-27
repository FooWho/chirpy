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

	"github.com/FooWho/chirpy/internal/auth"
	"github.com/FooWho/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

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

func databaseUserToAPIUser(dbUser database.User) apiUser {
	return apiUser{
		ID:             dbUser.ID,
		CreatedAt:      dbUser.CreatedAt,
		UpdatedAt:      dbUser.UpdatedAt,
		Email:          dbUser.Email,
		HashedPassword: "",
		Password:       "",
	}
}

type apiChirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func databaseChirpToAPIChirp(dbChirp database.Chirp) apiChirp {
	return apiChirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}
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

	mux.HandleFunc("POST /admin/reset", myCfg.doReset)
	mux.HandleFunc("POST /api/chirps", myCfg.createChirp)
	mux.HandleFunc("GET /api/chirps", myCfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{id}", myCfg.getChirpById)
	mux.HandleFunc("POST /api/users", myCfg.createUser)
	mux.HandleFunc("POST /api/login", myCfg.loginUser)
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
	return strings.Join(words, " ")
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {
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
		validChirp := databaseChirpToAPIChirp(dbChirp)
		respondWithJSON(w, 201, validChirp)
	} else {
		respondWithError(w, 400, "Chirp too long")
	}
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		respondWithError(w, 500, "Error getting chirps")
		return
	}
	apiChirps := make([]apiChirp, 0, len(chirps))
	for _, chirp := range chirps {
		apiChirps = append(apiChirps, databaseChirpToAPIChirp(chirp))
	}
	respondWithJSON(w, 200, apiChirps)
}

func (cfg *apiConfig) getChirpById(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	parsedId, err := uuid.Parse(id)
	if err != nil {
		log.Printf("Error parsing uuid: %s", err)
		respondWithError(w, 500, "Error parsing uuid")
		return
	}
	chirp, err := cfg.dbQueries.GetChirpById(r.Context(), parsedId)
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		respondWithError(w, 404, "Chirp not found")
		return
	}
	respondWithJSON(w, 200, databaseChirpToAPIChirp(chirp))
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
	user.HashedPassword, err = auth.HashPassword(user.Password)
	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{Email: user.Email, HashedPassword: user.HashedPassword})
	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithError(w, 500, user.Email)
		return
	}
	resp := databaseUserToAPIUser(dbUser)
	respondWithJSON(w, 201, resp)
}

func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	user := apiUser{}
	err := decoder.Decode(&user)
	if err != nil {
		log.Printf("Error logging in user: %s", err)
		respondWithError(w, 500, user.Email)
	}
	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), user.Email)
	if err != nil {
		log.Printf("Error logging in user: %s", err)
		respondWithError(w, 500, user.Email)
	}
	match, err := auth.CheckPasswordHash(user.Password, dbUser.HashedPassword)
	if err != nil {
		log.Printf("Error logging in user: %s", err)
		respondWithError(w, 500, user.Email)
	}
	if match {
		user = databaseUserToAPIUser(dbUser)
		log.Printf("User %s logged in with password %s", user.Email, user.Password)
		respondWithJSON(w, 200, user)
	} else {
		log.Printf("Bad password for user %s with password %s", user.Email, user.Password)
		respondWithError(w, 401, "Incorrect email or password")
	}
}

func (cfg *apiConfig) getMetrics(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Content-Type", "text/html")
	w.WriteHeader(200)
	hits := cfg.getHits()
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

func (apc *apiConfig) getHits() int {
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

func (cfg *apiConfig) GetSecret() ([]byte, error) {
	return []byte(cfg.tokenSecret), nil
}
