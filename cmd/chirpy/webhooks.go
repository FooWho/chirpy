package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/FooWho/chirpy/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) setUserToRed(w http.ResponseWriter, r *http.Request) {
	type upgradeParams struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	ptoken, err := auth.GetAPIKey(r.Header)
	if err != nil {
		log.Print("Request does not have Authorization header")
		respondWithError(w, http.StatusUnauthorized, "Missing Authorization Header")
		return
	}
	if ptoken != cfg.polkaKey {
		msg := "Wrong API key"
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
	}
	params := upgradeParams{}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&params)
	if err != nil {
		msg := fmt.Sprintf("Error decoding message body in setUserToRed: %s", err)
		log.Printf("Error decoding message body in setUserToRed: %s", msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	if params.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		msg := fmt.Sprintf("Error decoding user_id in setUserToRed: %s", err)
		log.Printf("Error decoding user_id in setUserToRed: %s", msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	_, err = cfg.dbQueries.SetUserToRed(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		msg := fmt.Sprintf("No such user: %s", params.Data.UserID)
		log.Print(msg)
		respondWithError(w, http.StatusNotFound, msg)
		return
	}
	if err != nil {
		msg := fmt.Sprintf("Error setting user to red in setUserToRed: %s", err)
		log.Printf("Error setting user to red in setUserToRed: %s", msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	w.WriteHeader(204)
}
