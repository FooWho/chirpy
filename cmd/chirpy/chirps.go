package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/FooWho/chirpy/internal/database"
	"github.com/google/uuid"
)

type apiChirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
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
		Body string `json:"body"`
	}
	userID, ok := r.Context().Value(userIDKey).(uuid.UUID)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "User ID not found in context")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Error decoding parameters")
		return
	}

	if len(params.Body) <= 140 {
		dbChirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   replaceBadWords(params.Body),
			UserID: userID,
		})
		if err != nil {
			log.Printf("Error creating chirp: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error creating chirp")
			return
		}
		validChirp := databaseChirpToAPIChirp(dbChirp)
		respondWithJSON(w, http.StatusCreated, validChirp)
	} else {
		respondWithError(w, http.StatusBadRequest, "Chirp too long")
	}
}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(uuid.UUID)
	if !ok {
		msg := "UserID not found in this context"
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		msg := fmt.Sprintf("Could not parse chirp ID: %s", r.PathValue("id"))
		log.Print(msg)
		respondWithError(w, http.StatusBadRequest, msg)
		return
	}
	chirp, err := cfg.dbQueries.GetChirpById(r.Context(), chirpID)
	if err != nil {
		msg := fmt.Sprintf("No such chirp: %s", chirpID)
		log.Print(msg)
		respondWithError(w, http.StatusNotFound, msg)
		return
	}
	if chirp.UserID != userID {
		msg := fmt.Sprintf("No chirp %s for %s", chirpID, userID)
		log.Print(msg)
		respondWithError(w, http.StatusForbidden, msg)
		return
	}
	err = cfg.dbQueries.DeleteChirp(r.Context(), database.DeleteChirpParams{
		ID:     chirpID,
		UserID: userID,
	})
	if err != nil {
		msg := fmt.Sprintf("Internal error - Chirp %s for %s exists, but was not deleted", chirpID, userID)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	author_id_str := r.URL.Query().Get("author_id")
	if author_id_str != "" {
		author_id, err := uuid.Parse(author_id_str)
		if err != nil {
			msg := fmt.Sprintf("Could not generate uuid for %s", author_id_str)
			log.Print(msg)
			respondWithError(w, http.StatusInternalServerError, msg)
			return
		}
		chirps, err := cfg.dbQueries.GetChirpsByAuthor(r.Context(), author_id)
		if err != nil {
			msg := fmt.Sprintf("Could not get chirps for %s", author_id)
			log.Print(msg)
			respondWithError(w, http.StatusInternalServerError, msg)
			return
		}
		apiChirps := make([]apiChirp, 0, len(chirps))
		for _, chirp := range chirps {
			apiChirps = append(apiChirps, databaseChirpToAPIChirp(chirp))
		}
		respondWithJSON(w, http.StatusOK, apiChirps)
		return
	} else {
		chirps, err := cfg.dbQueries.GetChirps(r.Context())
		if err != nil {
			log.Printf("Error getting chirps: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error getting chirps")
			return
		}
		apiChirps := make([]apiChirp, 0, len(chirps))
		for _, chirp := range chirps {
			apiChirps = append(apiChirps, databaseChirpToAPIChirp(chirp))
		}
		respondWithJSON(w, http.StatusOK, apiChirps)
		return
	}

}

func (cfg *apiConfig) getChirpById(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		log.Printf("Error parsing uuid: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Error parsing uuid")
		return
	}
	chirp, err := cfg.dbQueries.GetChirpById(r.Context(), chirpID)
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		respondWithError(w, http.StatusNotFound, "Chirp not found")
		return
	}
	respondWithJSON(w, http.StatusOK, databaseChirpToAPIChirp(chirp))
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
