package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

func (cfg *apiConfig) setUserToRed(w http.ResponseWriter, r *http.Request) {
	type upgradeParams struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	params := upgradeParams{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&params)
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
	dbuser, err := cfg.dbQueries.SetUserToRed(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		// user_id wasn't located
	}
	if err != nil {
		msg := fmt.Sprintf("Error setting user to red in setUserToRed: %s", err)
		log.Printf("Error setting user to red in setUserToRed: %s", msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

}

//{
//  "event": "user.upgraded",
//  "data": {
//    "user_id": "3311741c-680c-4546-99f3-fc9efac2036c"
//  }
//}
