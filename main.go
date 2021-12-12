package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"

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

func generateDeepCutPlaylist() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		playlistId := mux.Vars(r)["playlistId"]

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
		ctx := context.Background()
		spotifyPlaylistId := spotify.ID(playlistId)
		//Get input playlist
		playlist := getPlaylist(ctx, client, spotifyPlaylistId)
		fmt.Println("Playlist retrieved.")
		//Get each track
		originalTracks := getFullTracksFromPlaylist(ctx, client, playlist)
		fmt.Println("Tracks retrieved.")

		playlistName := playlist.Name
		newPlaylistName := playlistName + "-deep-cut"

		programMode := os.Getenv("PROGRAM_MODE")

		finalTracks := getFinalPlaylistTracks(ctx, client, originalTracks, programMode)
		//Create playlist
		generatedPlaylistId := createPlaylist(ctx, client, user, finalTracks, newPlaylistName)

		defer r.Body.Close()

		json.NewEncoder(w).Encode(generatedPlaylistId)

	}
}

func completeAuth(w http.ResponseWriter, r *http.Request) {
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

func getPlaylist(ctx context.Context, client *spotify.Client, playlistId spotify.ID) *spotify.FullPlaylist {
	fmt.Println("Beginning getPlaylist")
	finalPlaylist := spotify.FullPlaylist{}

	offset := 0
	limit := 100

	for {
		spotifyOffset := spotify.Offset(offset)
		playlist, err := client.GetPlaylist(ctx, playlistId, spotifyOffset, spotify.Limit(limit))

		if err != nil {
			fmt.Println(err.Error)
		}
		fmt.Println(playlist.ID)

		if finalPlaylist.Name == "" {
			finalPlaylist.Name = playlist.Name
		}
		finalPlaylist.Tracks.Tracks = append(finalPlaylist.Tracks.Tracks, playlist.Tracks.Tracks...)

		if playlist.Tracks.Total <= offset+limit {
			break
		}
		offset = offset + limit
	}

	return &finalPlaylist
}

func getFullTracksFromPlaylist(ctx context.Context, client *spotify.Client, playlist *spotify.FullPlaylist) []spotify.FullTrack {
	fmt.Println("Beginning getFullTracksFromPlaylist")
	tracks := []spotify.FullTrack{}
	playlistTracks := playlist.Tracks.Tracks
	for _, playlistTrack := range playlistTracks {
		tracks = append(tracks, playlistTrack.Track)
	}
	return tracks
}

func getAlbum(ctx context.Context, client *spotify.Client, albumId spotify.ID) *spotify.FullAlbum {
	fmt.Println("Beginning getAlbum")
	album, err := client.GetAlbum(ctx, albumId)

	if err != nil {
		fmt.Println(err.Error)
	}
	fmt.Println(album.ID)
	return album
}

func createPlaylist(ctx context.Context, client *spotify.Client, user *spotify.PrivateUser, tracks []spotify.SimpleTrack, playlistName string) string {
	fmt.Println("Beginning createPlaylist")

	playlistDescription := "Created automatically"
	createPublicPlaylist := false
	collaborativePlaylist := false
	userId := user.ID

	createdPlaylist, err := client.CreatePlaylistForUser(ctx, userId, playlistName, playlistDescription, createPublicPlaylist, collaborativePlaylist)
	if err != nil {
		fmt.Println(err.Error)
	}
	playlistId := createdPlaylist.ID
	//Get track IDs from list of tracks
	var trackIds []spotify.ID
	for _, track := range tracks {
		trackIds = append(trackIds, track.ID)
	}
	size := 100
	var j int

	//Batch trackIds in groups of 100 for being added to the playlist
	for i := 0; i < len(trackIds); i += size {
		j += size
		if j > len(trackIds) {
			j = len(trackIds)
		}
		fmt.Println(trackIds[i:j])
		_, err = client.AddTracksToPlaylist(ctx, playlistId, trackIds[i:j]...)
		if err != nil {
			fmt.Println(err.Error)
		}
	}

	fmt.Println("Playlist created")
	return playlistId.String()
}

func getFinalPlaylistTracks(ctx context.Context, client *spotify.Client, originalTracks []spotify.FullTrack, programMode string) []spotify.SimpleTrack {
	finalTracks := []spotify.SimpleTrack{}
	forbiddenSongs := []spotify.SimpleTrack{}

	//First we add every item on the list to the forbiddenSongs list
	for _, originalTrack := range originalTracks {
		forbiddenSongs = append(forbiddenSongs, originalTrack.SimpleTrack)
	}

	for i, originalTrack := range originalTracks {
		//Get album for each track, and the tracks from those albums
		albumId := originalTrack.Album.ID
		album := getAlbum(ctx, client, albumId)
		fmt.Println("Album retrieved: " + strconv.Itoa(i))
		albumTracklist := album.Tracks.Tracks
		switch programMode {
		case "ALL_BUT_ORIGINAL":
			//For each track on the album, add it to the final list if it isn't the original track
			for _, albumTrack := range albumTracklist {
				if !isSongIDForbidden(forbiddenSongs, albumTrack) {
					finalTracks = append(finalTracks, albumTrack)
				}
			}
		case "ONE_TRACK_PER_TRACK":
			//Go through the rest of the album's tracks and test for acceptable songs (which order?)
			//Figure out acceptable songs
			acceptableTracks := findAcceptableTracks(albumTracklist, forbiddenSongs)
			if len(acceptableTracks) > 0 {
				trackIndex := rand.Int() % len(acceptableTracks)
				finalTracks = append(finalTracks, albumTracklist[trackIndex])
				forbiddenSongs = append(forbiddenSongs, albumTracklist[trackIndex])
			}
		}
	}
	return finalTracks
}

func isSongIDForbidden(forbiddenSongs []spotify.SimpleTrack, song spotify.SimpleTrack) bool {
	return contains(forbiddenSongs, song)
}

func contains(s []spotify.SimpleTrack, e spotify.SimpleTrack) bool {
	for _, a := range s {
		if a.ID == e.ID {
			return true
		}
	}
	return false
}

func findAcceptableTracks(potentialTracks []spotify.SimpleTrack, forbiddenTracks []spotify.SimpleTrack) []spotify.SimpleTrack {
	acceptableTracks := []spotify.SimpleTrack{}
	//Loop through list One
	for _, potentialTrack := range potentialTracks {
		addTrack := true
		//Loop through list Two
		for _, forbiddenTrack := range forbiddenTracks {
			if potentialTrack.ID == forbiddenTrack.ID {
				//If an item in the potentialTrack exists in the forbiddenTrack list, do not add it to the acceptableTrack list.
				addTrack = false
				break
			}
		}
		if addTrack == true {
			acceptableTracks = append(acceptableTracks, potentialTrack)
		}
	}
	return acceptableTracks
}

type healthCheckResponse struct {
	Status string `json:"status"`
}

func health() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		response := healthCheckResponse{
			Status: "Ok",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

func handleRequests() {
	myRouter := mux.NewRouter().StrictSlash(true)
	myRouter.HandleFunc("/callback", completeAuth)
	myRouter.HandleFunc("/{playlistId}", generateDeepCutPlaylist()).Methods("POST")
	log.Fatal(http.ListenAndServe(":8080", myRouter))
}

func main() {
	fmt.Println("Listening on port 8080")
	handleRequests()
}
