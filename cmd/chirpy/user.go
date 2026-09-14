package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FooWho/chirpy/internal/auth"
	"github.com/FooWho/chirpy/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {
	type loginUserRequestParams struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type loginUserResponseParams struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		TokenJWT     string    `json:"token"`
		TokenRefresh string    `json:"refresh_token"`
	}

	decoder := json.NewDecoder(r.Body)
	params := loginUserRequestParams{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error logging in user: %s", err)
		respondWithError(w, http.StatusInternalServerError, params.Email)
		return
	}

	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		log.Printf("Error logging in user: %s", err)
		respondWithError(w, http.StatusUnauthorized, "User does not exist")
		return
	}

	match, err := auth.CheckPasswordHash(params.Password, dbUser.HashedPassword)
	if err != nil {
		log.Printf("Error logging in user: %s", err)
		respondWithError(w, http.StatusUnauthorized, "Password incorrect")
		return
	}
	if match {
		user := databaseUserToAPIUser(dbUser)

		token, err := auth.MakeJWT(user.ID, cfg.tokenSecret, time.Hour)
		if err != nil {
			log.Printf("Error creating token for user %s\n", user.Email)
			respondWithError(w, http.StatusInternalServerError, "Error creating token")
			return
		}
		log.Printf("User %s logged in", user.Email)

		refresh := auth.MakeRefreshToken()
		cfg.dbQueries.CreateRefreshToken(context.Background(), database.CreateRefreshTokenParams{
			Token:     refresh,
			UserID:    user.ID,
			ExpiresAt: time.Now().UTC().Add(24 * time.Hour * 60),
			RevokedAt: sql.NullTime{Valid: false},
		})

		loginUserResponse := loginUserResponseParams{
			ID:           user.ID,
			CreatedAt:    user.CreatedAt,
			UpdatedAt:    user.UpdatedAt,
			Email:        user.Email,
			TokenJWT:     token,
			TokenRefresh: refresh,
		}

		respondWithJSON(w, http.StatusOK, loginUserResponse)

	} else {
		log.Printf("Bad password for user %s with password %s", params.Email, params.Password)
		respondWithError(w, http.StatusForbidden, "Incorrect email or password")
	}
}

func (cfg *apiConfig) refreshUser(w http.ResponseWriter, r *http.Request) {
	type refreshUserResponse struct {
		Token string `json:"token"`
	}
	ptoken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Print("Request does not have Authorization header")
		respondWithError(w, http.StatusUnauthorized, "Missing Authorization Header")
		return
	}
	dbUser, err := cfg.dbQueries.GetUserByRefreshToken(context.Background(), ptoken)
	if err != nil {
		log.Print("No user located for this refresh token")
		respondWithError(w, http.StatusUnauthorized, "No user match for this refresh token")
		return
	}
	user := databaseUserToAPIUser(dbUser)

	token, err := auth.MakeJWT(user.ID, cfg.tokenSecret, time.Hour)
	if err != nil {
		log.Printf("Error creating token for user %s\n", user.Email)
		respondWithError(w, http.StatusInternalServerError, "Error created JWT")
		return
	}
	log.Printf("User %s access token refreshed", user.Email)
	respondWithJSON(w, http.StatusOK, refreshUserResponse{
		Token: token,
	})
}

func (cfg *apiConfig) revokeRefresh(w http.ResponseWriter, r *http.Request) {
	ptoken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Print("Request does not have Authorization header")
		respondWithError(w, http.StatusUnauthorized, "Missing Authorization Header")
		return
	}
	dbUser, err := cfg.dbQueries.GetUserByRefreshToken(context.Background(), ptoken)
	if err != nil {
		log.Print("No user located for this refresh token")
		respondWithError(w, http.StatusUnauthorized, "No user match for this refresh token")
		return
	}
	user := databaseUserToAPIUser(dbUser)
	err = cfg.dbQueries.UpdateRefreshToken(r.Context(), database.UpdateRefreshTokenParams{
		UpdatedAt: time.Now().UTC(),
		RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		Token:     ptoken,
	})
	if err != nil {
		msg := fmt.Sprintf("Could not revoke session for %s: %v", user.Email, err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return

	}
	log.Printf("Refresh token revoked for user %s", user.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	user := apiUser{}
	err := decoder.Decode(&user)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, http.StatusInternalServerError, user.Email)
		return
	}
	user.HashedPassword, err = auth.HashPassword(user.Password)
	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
	})
	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithError(w, http.StatusInternalServerError, user.Email)
		return
	}
	resp := databaseUserToAPIUser(dbUser)
	respondWithJSON(w, http.StatusCreated, resp)
}

func (cfg *apiConfig) updateUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(uuid.UUID)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "User ID not found in context")
		return
	}

	dbUser, err := cfg.dbQueries.GetUserById(r.Context(), userID)

	decoder := json.NewDecoder(r.Body)
	user := apiUser{}
	err = decoder.Decode(&user)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, http.StatusInternalServerError, user.Email)
		return
	}
	user.ID = dbUser.ID
	if user.Email == "" {
		user.Email = dbUser.Email
	}
	if user.Password == "" {
		user.HashedPassword = dbUser.HashedPassword
	} else {
		user.HashedPassword, err = auth.HashPassword(user.Password)
		if err != nil {
			log.Print("Error generating hashed password")
			respondWithError(w, http.StatusInternalServerError, "Error generating hashed password")
			return
		}
	}
	updatedUser, err := cfg.dbQueries.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
		ID:             user.ID,
	})
	if err != nil {
		log.Printf("Error updaing user %s", user.ID)
		respondWithError(w, http.StatusInternalServerError, "Error updating user")
		return
	}
	user = databaseUserToAPIUser(updatedUser)
	log.Printf("User %s updated - email: %s - HashedPassword: %s", user.ID, user.Email, user.HashedPassword)

	respondWithJSON(w, http.StatusOK, user)
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
