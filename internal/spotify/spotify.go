package spotify

import (
	"context"
	"fmt"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"math/rand"
	"net/http"

	"github.com/zmb3/spotify/v2"
	"time"
)

const redirectURI = "http://localhost:8080/callback"

var (
	auth = spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopePlaylistModifyPublic),
	)
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

func GetAuth() (*spotify.Client, *spotify.PrivateUser) {
	var client *spotify.Client

	rand.Seed(time.Now().UnixNano())
	url := auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// wait for auth to complete
	client = <-ch

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("You are logged in as:", user.ID)

	return client, user
}

func CompleteAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}
	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, state)
	}
	// use the token to get an authenticated client
	client := spotify.New(auth.Client(r.Context(), tok))
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}
