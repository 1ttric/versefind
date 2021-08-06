package pkg

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// Prevents duplicate records from being added
	syncTrackMutex sync.Mutex
	// Maps session ids to Spotify account information
	activeUsers sync.Map
	// Spotify authenticator object
	spotifyAuth spotify.Authenticator
	// The OAuth2 client ID for Spotify
	oauthClientID string
	// The OAuth2 client secret for Spotify
	oauthSecret string
	// Global Elasticsearch connection
	es *elasticsearch.Client
)

type GeniusSearchResult struct {
	Meta struct {
		Status int `json:"status"`
	} `json:"meta"`
	Response struct {
		Sections []struct {
			Type string `json:"type"`
			Hits []struct {
				Highlights []interface{} `json:"highlights"`
				Index      string        `json:"index"`
				Type       string        `json:"type"`
				Result     struct {
					Type                     string      `json:"_type"`
					AnnotationCount          int         `json:"annotation_count"`
					APIPath                  string      `json:"api_path"`
					FullTitle                string      `json:"full_title"`
					HeaderImageThumbnailURL  string      `json:"header_image_thumbnail_url"`
					HeaderImageURL           string      `json:"header_image_url"`
					ID                       int         `json:"id"`
					Instrumental             bool        `json:"instrumental"`
					LyricsOwnerID            int         `json:"lyrics_owner_id"`
					LyricsState              string      `json:"lyrics_state"`
					LyricsUpdatedAt          int         `json:"lyrics_updated_at"`
					Path                     string      `json:"path"`
					PyongsCount              interface{} `json:"pyongs_count"`
					SongArtImageThumbnailURL string      `json:"song_art_image_thumbnail_url"`
					SongArtImageURL          string      `json:"song_art_image_url"`
					Stats                    struct {
						UnreviewedAnnotations int  `json:"unreviewed_annotations"`
						Hot                   bool `json:"hot"`
					} `json:"stats"`
					Title             string `json:"title"`
					TitleWithFeatured string `json:"title_with_featured"`
					UpdatedByHumanAt  int    `json:"updated_by_human_at"`
					URL               string `json:"url"`
					PrimaryArtist     struct {
						Type           string `json:"_type"`
						APIPath        string `json:"api_path"`
						HeaderImageURL string `json:"header_image_url"`
						ID             int    `json:"id"`
						ImageURL       string `json:"image_url"`
						IndexCharacter string `json:"index_character"`
						IsMemeVerified bool   `json:"is_meme_verified"`
						IsVerified     bool   `json:"is_verified"`
						Name           string `json:"name"`
						Slug           string `json:"slug"`
						URL            string `json:"url"`
					} `json:"primary_artist"`
				} `json:"result"`
			} `json:"hits"`
		} `json:"sections"`
	} `json:"response"`
}

// A Versefind indexing progress report, for transmission to and display by the frontend
type UserProgress struct {
	Text     string `json:"text"`
	Complete bool   `json:"complete"`
	Total    int    `json:"total"`
	N        int    `json:"n"`
}

// A Versefind track combining Spotify data and scraped lyrics
type VerseTrack struct {
	Spotify spotify.FullTrack `json:"spotify"`
	Lyrics  string            `json:"lyrics"`
}

// A Versefind search result
type SearchResults struct {
	Total   int          `json:"total"`
	Results []VerseTrack `json:"results"`
}

// A result from an Elasticsearch query
type ElasticSearchResult struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore float64 `json:"max_score"`
		Hits     []struct {
			Index  string     `json:"_index"`
			Type   string     `json:"_type"`
			ID     string     `json:"_id"`
			Score  float64    `json:"_score"`
			Source VerseTrack `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type activeUser struct {
	progress           *UserProgress
	session            string
	wsMutex            sync.Mutex
	ws                 *websocket.Conn
	token              *oauth2.Token
	indexedTracks      sync.Map
	indexWaiter        *sync.WaitGroup
	searchableTrackIDs []string
}

func NewActiveUser(session string, token *oauth2.Token) *activeUser {
	return &activeUser{
		wsMutex:       sync.Mutex{},
		session:       session,
		token:         token,
		progress:      &UserProgress{Complete: true},
		indexedTracks: sync.Map{},
	}
}

func (u *activeUser) UseWebsocket(ws *websocket.Conn) {
	log.Tracef("UseWebsocket")
	u.wsMutex.Lock()
	defer u.wsMutex.Unlock()
	if u.ws != nil {
		log.Infof("replacing existing websocket")
		_ = u.ws.Close()
	}
	u.ws = ws
}

func (u *activeUser) SendProgress() error {
	log.Tracef("SendProgress")
	u.wsMutex.Lock()
	defer u.wsMutex.Unlock()
	log.Tracef("sending user progress: %+v", u.progress)
	return u.ws.WriteJSON(u.progress)
}

func (u *activeUser) SetProgress(n, total int, text string, complete bool) {
	u.progress.N = n
	u.progress.Total = total
	u.progress.Text = text
	u.progress.Complete = complete
}

func (u *activeUser) GetProgress() UserProgress {
	return *u.progress
}

func (u *activeUser) Index() {
	if u.indexWaiter != nil {
		u.indexWaiter.Wait()
		_ = u.SendProgress()
		return
	}

	u.indexWaiter = &sync.WaitGroup{}
	u.indexWaiter.Add(2)
	haltIndexing := false
	// Indexer worker. Fetches the user's Spotify tracks and then indexes them in Versefind.
	go func() {
		defer u.indexWaiter.Done()
		defer func() { haltIndexing = true }()

		// Fetch tracks from Spotify
		client := spotifyAuth.NewClient(u.token)
		log.Debugf("starting lyric collector")
		var spotifyTracks []spotify.FullTrack
		userTracks, err := client.CurrentUsersTracks()
		if err != nil {
			log.Errorf("could not fetch user's tracks: %s", err.Error())
			return
		}
		for {
			if haltIndexing {
				return
			}
			for pageIdx, userTrack := range userTracks.Tracks {
				spotifyTracks = append(spotifyTracks, userTrack.FullTrack)
				u.SetProgress(userTracks.Offset+pageIdx+1, userTracks.Total, "Indexing Spotify", false)
			}
			if errors.Is(err, spotify.ErrNoMorePages) {
				break
			}
			err = client.NextPage(userTracks)
		}

		// Remove user's already indexed tracks (if coming from an existing session)
		for idx := len(spotifyTracks) - 1; idx >= 0; idx-- {
			if _, ok := u.indexedTracks.Load(spotifyTracks[0].ID.String()); ok {
				spotifyTracks = append(spotifyTracks[:idx], spotifyTracks[idx+1:]...)
				idx--
			}
		}

		// Index lyrics
		for trackIdx, track := range spotifyTracks {
			if haltIndexing {
				return
			}
			err = IndexLyrics(track)
			if err != nil {
				log.Warnf("could not index lyrics for %s: %s", track.ID, err.Error())
				continue
			}
			u.indexedTracks.Store(track.ID.String(), track)
			u.SetProgress(trackIdx+1, len(spotifyTracks), "Indexing lyrics", false)
		}
	}()

	// Progress worker. Sends a user's progress to the frontend periodically
	go func() {
		defer u.indexWaiter.Done()
		defer func() { haltIndexing = true }()

		for {
			err := u.SendProgress()
			if err != nil {
				log.Debugf("could not send progress: %s", err.Error())
				return
			}
			if haltIndexing {
				return
			}
			time.Sleep(time.Millisecond * 250)
		}
	}()
	u.indexWaiter.Wait()
	log.Tracef("indexing complete")
	u.SetProgress(0, 0, "", true)
	_ = u.SendProgress()
	// Wait for the frontend to confirm receipt of the last progress report (or close the connection)
	_, _, _ = u.ws.ReadMessage()
}

// Initiates the Spotify OAuth2 flow
func authHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handling auth request for %s", r.RemoteAddr)
	stateBytes := make([]byte, 16)
	_, err := rand.Read(stateBytes)
	if err != nil {
		log.Errorf("could not generate session id: %s", err.Error())
		http.Error(w, "", 500)
		return
	}
	state := hex.EncodeToString(stateBytes)
	http.SetCookie(w, &http.Cookie{Name: "session", Value: state, Path: "/"})
	authUrl := spotifyAuth.AuthURL(state)
	log.Debugf("redirecting to auth url %s", authUrl)
	http.Redirect(w, r, authUrl, 302)
}

// OAuth2 callback
func callbackHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handling auth callback for %s", r.RemoteAddr)
	sessionCookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "invalid session", 403)
		return
	}
	session := sessionCookie.Value
	if session != r.URL.Query().Get("state") {
		http.Error(w, "invalid session", 403)
		return
	}
	token, err := spotifyAuth.Token(session, r)
	if err != nil {
		log.Errorf("could not generate oauth token: %s", err.Error())
		http.Error(w, "", 500)
		return
	}
	activeUsers.Store(session, NewActiveUser(session, token))
	http.Redirect(w, r, "/", 302)
}

// Index a user's tracks and send progress updates during the process
func wsHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("handling ws request from %s", r.RemoteAddr)
	upgrader := &websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return true
	}}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("could not upgrade websocket: %s", err.Error())
		return
	}

	// Ensure the user is authenticated
	user, err := getUserBySession(r)
	if err != nil {
		log.Errorf("could not get user: %s", err.Error())
		_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4000, "")) // If the user's session is not authenticated with Spotify, return a private use errror code via websocket to signal a reauth is needed
		return
	}
	user.UseWebsocket(ws)
	user.Index()
	log.Tracef("closing websocket")
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	// Attempt to retrieve a spotify API instance by this request's session cookie
	user, err := getUserBySession(r)
	if err != nil {
		log.Errorf("could not get spotify client: %s", err.Error())
		http.Error(w, "", 403)
		return
	}
	queryString := r.URL.Query().Get("q")
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, "", 400)
		return
	}
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil {
		http.Error(w, "", 400)
		return
	}

	log.Tracef("%s performing search with limit=%d, offset=%d for '%s'", r.RemoteAddr, limit, offset, queryString)

	var userTrackIds []string
	user.indexedTracks.Range(func(key, value interface{}) bool {
		userTrackIds = append(userTrackIds, key.(string))
		return true
	})

	// Short circuit if there are 0 tracks in the user's current indexed cache
	if len(userTrackIds) == 0 {
		log.Infof("short circuiting search")
		respBytes, err := json.Marshal(SearchResults{})
		if err != nil {
			log.Fatalf("could not marshal trimmed response: %s", err.Error())
		}
		_, _ = w.Write(respBytes)
		return
	}

	queryJson := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{
					map[string]interface{}{
						"ids": map[string]interface{}{
							"values": userTrackIds,
						},
					},
					map[string]interface{}{
						"query_string": map[string]interface{}{
							"query":            queryString,
							"analyze_wildcard": true,
							"default_operator": "AND",
						},
					},
				},
			},
		},
	}
	query, err := json.Marshal(queryJson)
	if err != nil {
		log.Fatalf("could not marshal query: %s", err.Error())
	}
	log.Debugf("raw query: %s", query)

	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	req := esapi.SearchRequest{
		Index:       []string{"tracks"},
		Body:        bytes.NewReader(query),
		TrackScores: &[]bool{true}[0],
		Size:        &limit,
		From:        &offset,
		Explain:     &[]bool{true}[0],
	}
	resp, err := req.Do(ctx, es)
	if err != nil {
		log.Fatalf("could not search elasticsearch: %s", err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("could read elasticsearch response: %s", err.Error())
	}
	if resp.IsError() && resp.StatusCode != 404 {
		log.Infof("could not search elasticsearch: status %d (%s)", resp.StatusCode, string(respBytes))
		http.Error(w, "", 400)
		return
	}
	var respJson ElasticSearchResult
	err = json.Unmarshal(respBytes, &respJson)
	if err != nil {
		log.Fatalf("elastic returned a non-JSON search result")
	}
	log.Debugf("search for '%s' returned %d results", queryString, respJson.Hits.Total.Value)

	var trimmedResp SearchResults
	trimmedResp.Total = respJson.Hits.Total.Value
	for _, hit := range respJson.Hits.Hits {
		trimmedResp.Results = append(trimmedResp.Results, hit.Source)
	}
	respBytes, err = json.Marshal(trimmedResp)
	if err != nil {
		log.Fatalf("could not marshal trimmed response: %s", err.Error())
	}
	_, _ = w.Write(respBytes)
}

// Uses the session cookie in an HTTP request to retrieve an active Spotify API instance using 'activeUsers'
func getUserBySession(r *http.Request) (*activeUser, error) {
	sessionCookie, err := r.Cookie("session")
	if err != nil {
		return nil, fmt.Errorf("could not get session cookie: %w", err)
	}
	ret, ok := activeUsers.Load(sessionCookie.Value)
	if !ok {
		return nil, errors.New("session cookie did not exist")
	}
	return ret.(*activeUser), nil
}

// Check whether this track is already indexed in Elastic
//noinspection GoNilness
func elasticTrackExists(spotifyId string) bool {
	queryObj := map[string]interface{}{
		"query": map[string]interface{}{
			"ids": map[string]interface{}{
				"values": []string{spotifyId},
			},
		},
	}

	query, err := json.Marshal(queryObj)
	if err != nil {
		log.Fatalf("could not marshal elastic query to check existing track: %s", err.Error())
	}
	req := esapi.SearchRequest{
		Index: []string{"tracks"},
		Body:  bytes.NewReader(query),
	}

	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	respObj, err := req.Do(ctx, es)
	if err != nil {
		log.Warnf("could not query elastic for existing track: %s", err.Error())
	}
	defer func() { _ = respObj.Body.Close() }()
	respBytes, err := ioutil.ReadAll(respObj.Body)
	if err != nil {
		log.Fatalf("could not read response from elasticsearch: %s", err.Error())
	}
	if respObj.IsError() && respObj.StatusCode != 404 {
		log.Fatalf("could not search elasticsearch: status %d (%s)", respObj.StatusCode, string(respBytes))
	}

	var respJson ElasticSearchResult
	err = json.Unmarshal(respBytes, &respJson)
	if err != nil {
		log.Fatalf("elastic returned a non-JSON search result")
	}
	return respJson.Hits.Total.Value > 0
}

// Given a track, scrape Genius/AZLyrics if any are present, then index the object in Elasticsearch
//noinspection GoNilness
func IndexLyrics(track spotify.FullTrack) error {
	syncTrackMutex.Lock()
	defer syncTrackMutex.Unlock()
	// Check whether the track is already in Elastic
	if elasticTrackExists(track.ID.String()) {
		return nil
	}

	// Prepare the search string with which to query the scrapers
	var artistNames []string
	for _, artist := range track.Artists {
		artistNames = append(artistNames, artist.Name)
	}
	query := track.Name + " " + strings.Join(artistNames, " ")
	log.Debugf("%s using query: %s", track.ID, query)

	// Search for this track in Genius and AZLyrics
	lyrics, exists, err := ScrapeGenius(query)
	if err != nil {
		return fmt.Errorf("could not scrape lyrics from genius: %w", err)
	}
	if !exists {
		log.Warnf("no genius lyrics were found for %s - trying azlyrics", track.ID)
		lyrics, exists, err = ScrapeAZLyrics(query)
		if err != nil {
			return fmt.Errorf("could not scrape lyrics from azlyrics: %w", err)
		}
	}
	if !exists {
		log.Warnf("no azlyrics lyrics were found for %s - defaulting to empty", track.ID)
		lyrics = ""
	}

	// Marshal and insert into Elasticsearch
	jsonDoc, err := json.Marshal(VerseTrack{track, lyrics})
	if err != nil {
		log.Fatalf("could not marshal doc for elasticsearch: %s", err.Error())
	}
	req := esapi.IndexRequest{
		DocumentID: track.ID.String(),
		Index:      "tracks",
		Body:       bytes.NewReader(jsonDoc),
		Refresh:    "wait_for",
	}
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	resp, err := req.Do(ctx, es)
	if err != nil {
		log.Fatalf("could not index document in elasticsearch: %s", err.Error())
	}
	if resp.IsError() {
		log.Fatalf("could not index document in elasticsearch: status %d", resp.StatusCode)
	}

	return nil
}

func ScrapeGenius(query string) (string, bool, error) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	params := url.Values{}
	params.Set("q", query)
	u := &url.URL{Scheme: "https", Host: "genius.com", Path: "/api/search/multi", RawQuery: params.Encode()}
	req, err := http.NewRequestWithContext(context.Background(), "GET", u.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("could not construct http request: %w", err)
	}
	req.Header.Set("User-Agent", "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("could not perform http request: %w", err)
	}
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("could not read response body: %w", err)
	}
	var respJson GeniusSearchResult
	err = json.Unmarshal(respData, &respJson)
	if err != nil {
		return "", false, fmt.Errorf("could not unmarshal response data: %w", err)
	}
	var link string

Outer:
	for _, section := range respJson.Response.Sections {
		for _, hit := range section.Hits {
			// Ensure this isn't a Genius or Spotify listicle instead of lyrics
			if hit.Index != "song" || hit.Result.PrimaryArtist.Name == "Genius" || hit.Result.PrimaryArtist.Name == "Spotify" {
				continue
			}
			link = hit.Result.Path
			break Outer
		}
	}
	if link == "" {
		return "", false, nil
	}

	// Scrape the lyrics from the found track page
	ctx, _ = context.WithTimeout(context.Background(), time.Second*5)
	u = &url.URL{Scheme: "https", Host: "genius.com", Path: link}
	req, err = http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("could not construct http request: %w", err)
	}
	req.Header.Set("User-Agent", "")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("could not perform http request: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("could not read response body: %w", err)
	}
	tag := doc.Find("div.lyrics")
	lyrics := strings.TrimSpace(tag.Text())
	if tag.Length() == 0 || lyrics == "" {
		return "", false, errors.New("page does not contain lyrics")
	}

	return lyrics, true, nil
}

func ScrapeAZLyrics(query string) (string, bool, error) {
	// Search for the lyrics
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	params := url.Values{}
	params.Set("q", query)
	u := &url.URL{Scheme: "https", Host: "search.azlyrics.com", Path: "/search.php", RawQuery: params.Encode()}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("could not construct http request: %w", err)
	}
	req.Header.Set("User-Agent", "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("could not perform http request with %s: %w", u.String(), err)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("could not read response body: %w", err)
	}
	link, exists := doc.Find("table a[href]").First().Attr("href")
	if !exists {
		return "", false, nil
	}

	// Scrape the lyrics
	ctx, _ = context.WithTimeout(context.Background(), time.Second*5)
	req, err = http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return "", false, fmt.Errorf("could not construct http request: %w", err)
	}
	req.Header.Set("User-Agent", "")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("could not perform http request: %w", err)
	}
	doc, err = goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("could not read response body: %w", err)
	}
	lyrics := doc.Find("div.main-page > div.row > div.text-center > div:nth-of-type(5)").First().Text()
	lyrics = strings.TrimSpace(lyrics)
	return lyrics, true, nil
}

// The main entrypoint to serve a Versefind API instance. Specify a listen address, an oauth redirect URL, and an
// Elasticsearch instance address
func Serve(listenAddr, oauthRedirectAddr, esAddr string) {
	activeUsers = sync.Map{}
	syncTrackMutex = sync.Mutex{}
	log.SetLevel(log.TraceLevel)
	log.SetReportCaller(true)
	log.Infof("versefind api starting")

	oauthClientID = os.Getenv("OAUTH_CLIENTID")
	oauthSecret = os.Getenv("OAUTH_SECRET")

	if oauthClientID == "" || oauthSecret == "" {
		log.Fatalf("Spotify OAuth2 credentials are required. Specify with OAUTH_CLIENTID and OAUTH_SECRET.")
	}

	cfg := elasticsearch.Config{
		Addresses: []string{
			esAddr,
		},
	}
	var err error
	es, err = elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("unable to initialize elasticsearch connection: %s", err.Error())
	}
	//noinspection GoNilness
	_, err = es.Ping()
	if err != nil {
		log.Fatalf("unable to connect to elasticsearch: %s", err.Error())
	}
	log.Infof("connected to elasticsearch")

	spotifyAuth = spotify.NewAuthenticator(oauthRedirectAddr, spotify.ScopeUserLibraryRead)
	spotifyAuth.SetAuthInfo(oauthClientID, oauthSecret)

	http.HandleFunc("/api/auth", authHandler)
	http.HandleFunc("/api/callback", callbackHandler)
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/api/search", searchHandler)
	_ = http.ListenAndServe(listenAddr, nil)
}
