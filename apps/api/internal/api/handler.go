package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/locationproof"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
)

type Handler struct {
	store              PlaceStore
	media              MediaUploadStore
	social             SocialRepository
	echoes             EchoStore
	profiles           ProfileStore
	authorizations     CreationAuthorizationStore
	engagement         EngagementRepository
	discovery          DiscoveryRepository
	account            AccountRepository
	admin              AdminRepository
	appeals            AppealRepository
	creationSigningKey []byte
	publicLocationKey  []byte
	clock              clock
}

type HandlerConfig struct {
	Media              MediaUploadStore
	Echoes             EchoStore
	Profiles           ProfileStore
	Authorizations     CreationAuthorizationStore
	Social             SocialRepository
	Engagement         EngagementRepository
	Discovery          DiscoveryRepository
	Account            AccountRepository
	Admin              AdminRepository
	Appeals            AppealRepository
	CreationSigningKey []byte
	PublicLocationKey  []byte
	Clock              clock
}

// creationBoundaryStore lets the production repository apply the authoritative
// PostGIS Place boundary. Test and local-memory stores retain the reviewed
// radius fallback used for point-only Places.
type creationBoundaryStore interface {
	CreationLocationAllowed(context.Context, string, float64, float64) (bool, error)
}

type publicLocationBoundaryStore interface {
	ConstrainPublicLocation(context.Context, string, Coordinate) (Coordinate, error)
}

func NewHandler(store PlaceStore) http.Handler {
	return NewHandlerWithConfig(store, HandlerConfig{})
}

func NewHandlerWithMedia(store PlaceStore, media MediaUploadStore) http.Handler {
	return NewHandlerWithConfig(store, HandlerConfig{Media: media})

}

func NewHandlerWithConfig(store PlaceStore, config HandlerConfig) http.Handler {
	if store == nil {
		store = NewStore(DefaultPlaces())
	}
	if config.Media == nil {
		config.Media = NewMemoryMediaUploadStore()
	}
	if config.Echoes == nil {
		config.Echoes = NewMemoryEchoStore(DefaultEchoes())
	}
	if config.Profiles == nil {
		config.Profiles = NewMemoryProfileStore()
	}
	if config.Authorizations == nil {
		config.Authorizations = NewMemoryCreationAuthorizationStore()
	}
	if config.Social == nil {
		authors := map[string]string{}
		for _, echo := range DefaultEchoes() {
			authors[echo.ID] = echo.AuthorUserID
		}
		config.Social = NewMemorySocialRepository(NewSocialStoreWithEchoAuthors(authors))
	}
	if config.Engagement == nil {
		config.Engagement = NewMemoryEngagementRepository()
	}
	if config.Discovery == nil {
		config.Discovery = NewMemoryDiscoveryRepository(store, config.Echoes)
	}
	if config.Account == nil {
		config.Account = NewMemoryAccountRepository()
	}
	if config.Appeals == nil {
		config.Appeals = NewMemoryAppealRepository()
	}
	if len(config.CreationSigningKey) == 0 {
		config.CreationSigningKey = []byte("echotrails-test-creation-key")
	}
	if len(config.PublicLocationKey) == 0 {
		config.PublicLocationKey = []byte("echotrails-test-public-location-key")
	}
	if config.Clock == nil {
		config.Clock = realClock{}
	}
	return &Handler{
		store: store, media: config.Media, social: config.Social, echoes: config.Echoes,
		profiles: config.Profiles, authorizations: config.Authorizations, creationSigningKey: config.CreationSigningKey,
		engagement: config.Engagement, discovery: config.Discovery, account: config.Account, admin: config.Admin, appeals: config.Appeals, publicLocationKey: config.PublicLocationKey, clock: config.Clock,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.enforceMutationIdempotency(w, r, h.serveHTTP) {
		return
	}
	h.serveHTTP(w, r)
}

func (h *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/admin/"):
		h.handleAdmin(w, r)
	case r.URL.Path == "/appeals" || strings.HasPrefix(r.URL.Path, "/appeals/"):
		h.handleAppeals(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/creation-tokens":
		h.handleCreationToken(w, r)
	case r.URL.Path == "/beta/access":
		h.handleBetaAccess(w, r)
	case r.URL.Path == "/consent/current" || r.URL.Path == "/consent/acceptances":
		h.handleConsent(w, r)
	case strings.HasPrefix(r.URL.Path, "/privacy/"):
		h.handlePrivacy(w, r)
	case r.URL.Path == "/analytics/opt-out" || r.URL.Path == "/analytics/events":
		h.handleAnalytics(w, r)
	case r.URL.Path == "/account/sessions" || strings.HasPrefix(r.URL.Path, "/account/sessions/"):
		h.handleAccountSessions(w, r)
	case r.URL.Path == "/account/bootstrap":
		h.handleAccountBootstrap(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/places/search":
		h.handleSearch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/places/requests":
		h.handleRequest(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/media/uploads":
		h.handleCreateMediaUpload(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/me/avatar/uploads":
		h.handleCreateAvatarUpload(w, r)
	case (r.Method == http.MethodPut || r.Method == http.MethodDelete) && r.URL.Path == "/me/avatar":
		h.handleAvatarSelection(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/media/assets/"):
		h.handlePublicMediaAsset(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/echoes/") && strings.HasSuffix(r.URL.Path, "/media/uploads"):
		h.handleCreateMediaUpload(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/media/uploads/"):
		h.handleGetMediaUpload(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/retry") && strings.HasPrefix(r.URL.Path, "/media/uploads/"):
		h.handleRenewMediaUpload(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete") && strings.HasPrefix(r.URL.Path, "/media/uploads/"):
		h.handleCompleteMediaUpload(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/discovery/nearby":
		h.handleNearby(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/discovery/notable":
		h.handleNotable(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/search":
		h.handleDiscoverySearch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/echoes":
		h.handleCreateEcho(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/echoes") && strings.HasPrefix(r.URL.Path, "/places/"):
		h.handlePlaceEchoes(w, r)
	case strings.HasPrefix(r.URL.Path, "/places/") && strings.HasSuffix(r.URL.Path, "/subscription"):
		h.handlePlaceSubscription(w, r)
	case r.URL.Path == "/subscriptions":
		h.handleSubscriptions(w, r)
	case r.URL.Path == "/devices" || strings.HasPrefix(r.URL.Path, "/devices/"):
		h.handleDevices(w, r)
	case r.URL.Path == "/notification-preferences":
		h.handleNotificationPreferences(w, r)
	case strings.HasPrefix(r.URL.Path, "/mutes/"):
		h.handleMute(w, r)
	case strings.HasPrefix(r.URL.Path, "/echoes/") && strings.HasSuffix(r.URL.Path, "/reactions"):
		h.handleEchoReactions(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/echoes/") && strings.HasSuffix(r.URL.Path, "/views"):
		h.handleEchoDetailView(w, r)
	case strings.HasPrefix(r.URL.Path, "/echoes/") && strings.HasSuffix(r.URL.Path, "/replies"):
		h.handleEchoReplies(w, r)
	case strings.HasPrefix(r.URL.Path, "/echoes/") && strings.HasSuffix(r.URL.Path, "/stitches"):
		h.handleEchoStitch(w, r)
	case strings.HasPrefix(r.URL.Path, "/replies/"):
		h.handleReplyResource(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/reports":
		h.handleCreateReport(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/hides":
		h.handleHide(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/blocks":
		h.handleBlock(w, r)
	case r.URL.Path == "/notifications":
		h.handleNotifications(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/echoes/") && (strings.HasSuffix(r.URL.Path, "/submit") || strings.HasSuffix(r.URL.Path, "/publish")):
		h.handleSubmitEcho(w, r)
	case strings.HasPrefix(r.URL.Path, "/echoes/"):
		h.handleEchoResource(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/places/"):
		h.handleDetail(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleEchoResource(w http.ResponseWriter, r *http.Request) {
	echoID := strings.TrimPrefix(r.URL.Path, "/echoes/")
	if !validSocialID(echoID) || strings.Contains(echoID, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.handleGetEcho(w, r)
	case http.MethodPatch:
		userID, _, ok := authenticatedUser(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var patch EchoPatchRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&patch); err != nil {
			writeEchoError(w, ErrInvalidEcho)
			return
		}
		record, err := h.echoes.UpdateDraft(r.Context(), echoID, userID, patch, h.clock.Now())
		if err != nil {
			writeEchoError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toEchoResponse(record, userID))
	case http.MethodDelete:
		userID, _, ok := authenticatedUser(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if err := h.echoes.Delete(r.Context(), echoID, userID, h.clock.Now()); err != nil {
			writeEchoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleRenewMediaUpload(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	uploadID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/media/uploads/"), "/retry")
	if uploadID == "" || strings.Contains(uploadID, "/") {
		http.NotFound(w, r)
		return
	}
	status, err := h.media.RenewUpload(r.Context(), userID, uploadID, h.clock.Now())
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

type creationTokenRequest struct {
	PlaceID        string    `json:"place_id"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	AccuracyMeters float64   `json:"accuracy_meters"`
	CapturedAt     time.Time `json:"captured_at"`
}

func (h *Handler) handleCreationToken(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	allowed, err := h.account.CreationAllowed(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "invite_required", "Beta invite required")
		return
	}
	var request creationTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_location_proof", "invalid location proof")
		return
	}
	place, found, err := h.store.GetPlace(r.Context(), strings.TrimSpace(request.PlaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !found || place.Latitude == nil || place.Longitude == nil || (place.State != PlaceStateActive && place.State != PlaceStateRestricted) {
		writeCreationTokenError(w, locationproof.ErrInvalidRequest)
		return
	}
	status := locationproof.PlaceActive
	if place.State == PlaceStateRestricted {
		status = locationproof.PlaceRestricted
	}
	center := locationproof.Coordinate{Latitude: *place.Latitude, Longitude: *place.Longitude}
	radius := place.RadiusMeters
	if radius <= 0 {
		radius = 100
	}
	if boundaries, ok := h.store.(creationBoundaryStore); ok && status == locationproof.PlaceActive {
		allowed, boundaryErr := boundaries.CreationLocationAllowed(r.Context(), place.ID, request.Latitude, request.Longitude)
		if boundaryErr != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if !allowed {
			writeCreationTokenError(w, locationproof.ErrOutsideBoundary)
			return
		}
		// Location age/accuracy/token validation remains centralized in the
		// locationproof package; containment has already been authoritatively
		// evaluated by PostGIS.
		center = locationproof.Coordinate{Latitude: request.Latitude, Longitude: request.Longitude}
		radius = 1
	}
	now := h.clock.Now()
	token, err := locationproof.ValidateCreationTokenRequest(locationproof.Request{
		UserID: userID, SelectedPlace: strings.TrimSpace(request.PlaceID),
		Proof:  locationproof.LocationProof{Coordinate: locationproof.Coordinate{Latitude: request.Latitude, Longitude: request.Longitude}, AccuracyMeters: request.AccuracyMeters, CapturedAt: request.CapturedAt},
		Places: []locationproof.Place{{ID: place.ID, Status: status, Center: center, RadiusMeters: radius}},
		Trust:  locationproof.GateDecision{Allowed: true}, Rate: locationproof.GateDecision{Allowed: true},
	}, locationproof.Config{Now: now, SigningKey: h.creationSigningKey})
	if err != nil {
		writeCreationTokenError(w, err)
		return
	}
	authorization := CreationAuthorization{
		TokenHash: hashCreationToken(token.Token), UserID: userID, PlaceID: token.PlaceID,
		Coordinate: Coordinate{Lat: request.Latitude, Lon: request.Longitude}, CapturedAt: request.CapturedAt.UTC(), AccuracyMeters: request.AccuracyMeters, ExpiresAt: token.ExpiresAt,
	}
	authorization, err = h.authorizations.Save(r.Context(), authorization)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"creation_token": token.Token, "place_id": token.PlaceID, "expires_at": token.ExpiresAt})
}

func (h *Handler) handleCreateEcho(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return
	}
	var request CreateEchoRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&request); err != nil {
		writeEchoError(w, ErrInvalidEcho)
		return
	}
	request.PlaceID = strings.TrimSpace(request.PlaceID)
	request.Title = strings.TrimSpace(request.Title)
	request.BodyText = strings.TrimSpace(request.BodyText)
	request.CaptionText = strings.TrimSpace(request.CaptionText)
	request.CreationToken = strings.TrimSpace(request.CreationToken)
	if err := ValidateCreateEchoRequest(request); err != nil {
		writeEchoError(w, err)
		return
	}
	place, found, err := h.store.GetPlace(r.Context(), request.PlaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !found || place.State != PlaceStateActive {
		writeError(w, http.StatusBadRequest, "place_unavailable", "Place is unavailable for creation")
		return
	}
	now := h.clock.Now()
	if _, err := locationproof.VerifyCreationToken(request.CreationToken, userID, request.PlaceID, now, h.creationSigningKey); err != nil {
		writeEchoError(w, ErrCreationProofMissing)
		return
	}
	authorization, err := h.authorizations.Consume(r.Context(), request.CreationToken, userID, request.PlaceID, idempotencyKey, now)
	if err != nil {
		writeEchoError(w, err)
		return
	}
	profile, found, err := h.profiles.GetProfileByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !found {
		writeError(w, http.StatusConflict, "profile_required", "Create a public profile before posting")
		return
	}
	publicLocation, err := h.publicLocationForPlace(r.Context(), place, hashCreationToken(request.CreationToken))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	record, err := h.echoes.CreateDraft(r.Context(), EchoCreateInput{
		Request: request, AuthorUserID: userID, AuthorHandle: profile.Handle, PrivateLocation: authorization.Coordinate,
		PublicLocation: publicLocation, CreationAuthorizationID: authorization.ID, IdempotencyKey: idempotencyKey, Now: now,
	})
	if err != nil {
		writeEchoError(w, err)
		return
	}
	if err := h.social.RegisterEcho(r.Context(), record.ID, record.AuthorUserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, toEchoResponse(record, userID))
}

func (h *Handler) publicLocationForPlace(ctx context.Context, place Place, seed string) (Coordinate, error) {
	candidate := jitteredPublicLocation(place, seed, h.publicLocationKey)
	if boundaries, ok := h.store.(publicLocationBoundaryStore); ok {
		return boundaries.ConstrainPublicLocation(ctx, place.ID, candidate)
	}
	return candidate, nil
}

func (h *Handler) handleGetEcho(w http.ResponseWriter, r *http.Request) {
	echoID := strings.TrimPrefix(r.URL.Path, "/echoes/")
	if !validSocialID(echoID) || strings.Contains(echoID, "/") {
		http.NotFound(w, r)
		return
	}
	requesterID, _, _ := authenticatedUser(r)
	record, err := h.echoes.Get(r.Context(), echoID, requesterID)
	if err != nil {
		writeEchoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEchoResponse(record, requesterID))
}

func (h *Handler) handleSubmitEcho(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	echoID := strings.TrimPrefix(r.URL.Path, "/echoes/")
	echoID = strings.TrimSuffix(strings.TrimSuffix(echoID, "/submit"), "/publish")
	if !validSocialID(echoID) || strings.Contains(echoID, "/") {
		http.NotFound(w, r)
		return
	}
	record, err := h.echoes.Submit(r.Context(), echoID, userID, h.clock.Now())
	if err != nil {
		writeEchoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEchoResponse(record, userID))
}

func (h *Handler) handleCreateMediaUpload(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req CreateMediaUploadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid media upload request")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/echoes/") {
		echoID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/echoes/"), "/media/uploads")
		if !validSocialID(echoID) || strings.Contains(echoID, "/") || (req.EchoID != "" && req.EchoID != echoID) {
			writeMediaError(w, ErrInvalidMediaUpload)
			return
		}
		req.EchoID = echoID
	}
	created, err := h.media.CreateUpload(r.Context(), userID, req, strings.TrimSpace(r.Header.Get("Idempotency-Key")), time.Now().UTC())
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) handleCreateAvatarUpload(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	store, ok := h.media.(AvatarMediaUploadStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "avatar_upload_unavailable", "avatar uploads are unavailable")
		return
	}
	var request CreateAvatarUploadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&request); err != nil {
		writeMediaError(w, ErrInvalidMediaUpload)
		return
	}
	created, err := store.CreateAvatarUpload(r.Context(), userID, request, strings.TrimSpace(r.Header.Get("Idempotency-Key")), h.clock.Now())
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) handleAvatarSelection(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodDelete {
		profile, err := h.profiles.ClearAvatar(r.Context(), userID, h.clock.Now())
		if err != nil {
			writeAvatarError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toMeProfile(profile, h.clock.Now()))
		return
	}
	var request struct {
		MediaAssetID string `json:"media_asset_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request); err != nil || !validSocialID(request.MediaAssetID) || strings.Contains(request.MediaAssetID, "/") {
		writeError(w, http.StatusBadRequest, "invalid_avatar", "invalid avatar selection")
		return
	}
	media, ok := h.media.(AvatarMediaUploadStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "avatar_upload_unavailable", "avatar uploads are unavailable")
		return
	}
	if err := media.ValidateAvatarSelection(r.Context(), userID, request.MediaAssetID); err != nil {
		writeAvatarError(w, err)
		return
	}
	profile, err := h.profiles.SetAvatar(r.Context(), userID, request.MediaAssetID, h.clock.Now())
	if err != nil {
		writeAvatarError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMeProfile(profile, h.clock.Now()))
}

func (h *Handler) handleGetMediaUpload(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	uploadID := strings.TrimPrefix(r.URL.Path, "/media/uploads/")
	if strings.Contains(uploadID, "/") || uploadID == "" {
		http.NotFound(w, r)
		return
	}
	status, err := h.media.GetUpload(r.Context(), userID, uploadID)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleCompleteMediaUpload(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	uploadID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/media/uploads/"), "/complete")
	if uploadID == "" {
		http.NotFound(w, r)
		return
	}
	status, err := h.media.CompleteUpload(r.Context(), userID, uploadID)
	if err != nil {
		writeMediaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

type publicMediaStore interface {
	GetPublicAssetURL(context.Context, string) (string, error)
}

func (h *Handler) handlePublicMediaAsset(w http.ResponseWriter, r *http.Request) {
	assetID := strings.TrimPrefix(r.URL.Path, "/media/assets/")
	store, ok := h.media.(publicMediaStore)
	if !ok || !validSocialID(assetID) || strings.Contains(assetID, "/") {
		http.NotFound(w, r)
		return
	}
	location, err := store.GetPublicAssetURL(r.Context(), assetID)
	if errors.Is(err, ErrMediaUploadNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	http.Redirect(w, r, location, http.StatusTemporaryRedirect)
}

func (h *Handler) handleNearby(w http.ResponseWriter, r *http.Request) {
	lat, latErr := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, lonErr := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if latErr != nil || lonErr != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		writeError(w, http.StatusBadRequest, "invalid_coordinates", "valid lat and lon are required")
		return
	}
	radius, ok := parseOptionalInt(r.URL.Query().Get("radius_meters"), 1000)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_radius", "radius_meters must be an integer")
		return
	}
	if radius < 10 || radius > 10000 {
		writeError(w, http.StatusBadRequest, "invalid_radius", "radius_meters must be between 10 and 10000")
		return
	}
	includeRestricted := r.URL.Query().Get("include_restricted") == "true"
	places, err := h.store.NearbyPlaces(r.Context(), lat, lon, radius, includeRestricted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": places})
}

func (h *Handler) handlePlaceEchoes(w http.ResponseWriter, r *http.Request) {
	placeID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/places/"), "/echoes")
	if p, ok, err := h.store.GetPlace(r.Context(), placeID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	} else if !ok || p.State != PlaceStateActive {
		http.NotFound(w, r)
		return
	}
	order := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if order == "" {
		order = "notable"
	}
	if order != "recent" && order != "notable" && order != "oldest" {
		writeError(w, http.StatusBadRequest, "invalid_sort", "Sort must be recent, notable, or oldest")
		return
	}
	viewerID, _, _ := authenticatedUser(r)
	var page EchoRecordPage
	var err error
	if visibleStore, ok := h.echoes.(interface {
		ListVisiblePublishedByPlaceSortedPage(context.Context, string, string, string, int, string) (EchoRecordPage, error)
	}); ok {
		page, err = visibleStore.ListVisiblePublishedByPlaceSortedPage(r.Context(), placeID, viewerID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20), order)
	} else {
		page, err = h.echoes.ListPublishedByPlaceSortedPage(r.Context(), placeID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20), order)
	}
	if err != nil {
		writePaginationError(w, err)
		return
	}
	echoes := make([]EchoResponse, 0, len(page.Records))
	for _, record := range page.Records {
		echoes = append(echoes, toEchoResponse(record, ""))
	}
	writeJSON(w, http.StatusOK, map[string]any{"echoes": echoes, "next_cursor": page.NextCursor})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = r.URL.Query().Get("query")
	}
	includeRestricted := r.URL.Query().Get("include_restricted") == "true"
	var latitude, longitude *float64
	if rawLat, rawLon := r.URL.Query().Get("lat"), r.URL.Query().Get("lon"); rawLat != "" || rawLon != "" {
		lat, latErr := strconv.ParseFloat(rawLat, 64)
		lon, lonErr := strconv.ParseFloat(rawLon, 64)
		if latErr != nil || lonErr != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			writeError(w, http.StatusBadRequest, "invalid_coordinates", "valid lat and lon are required")
			return
		}
		latitude, longitude = &lat, &lon
	}
	places, err := h.store.SearchPlaces(r.Context(), q, includeRestricted, latitude, longitude)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": places})
}

func (h *Handler) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/places/")
	p, ok, err := h.store.GetPlace(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch p.State {
	case PlaceStateActive, PlaceStateRestricted, PlaceStatePendingReview:
		writeJSON(w, http.StatusOK, p)
	case PlaceStateMerged:
		w.Header().Set("Location", "/places/"+p.CanonicalID)
		w.WriteHeader(http.StatusPermanentRedirect)
	case PlaceStateRejected:
		writeJSON(w, http.StatusGone, map[string]any{"id": p.ID, "state": p.State, "replacement_id": p.ReplacementID, "canonical_id": p.CanonicalID, "rejection_reason": p.RejectionReason})
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleRequest(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req PlaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" || req.Latitude == nil || req.Longitude == nil {
		writeError(w, http.StatusBadRequest, "invalid_place_request", "valid Place name and coordinates are required")
		return
	}
	p, err := h.store.CreatePlaceRequest(r.Context(), userID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, title string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "title": title}})
}

func writePaginationError(w http.ResponseWriter, err error) {
	if errors.Is(err, pagination.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
}

func writeMediaError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidMediaUpload):
		writeError(w, http.StatusBadRequest, "invalid_media_upload", "invalid media upload")
	case errors.Is(err, ErrMediaUploadNotFound):
		writeError(w, http.StatusNotFound, "media_upload_not_found", "media upload not found")
	case errors.Is(err, ErrMediaUploadExpired):
		writeError(w, http.StatusBadRequest, "media_upload_expired", "media upload expired")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeAvatarError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAvatarNotApproved):
		writeError(w, http.StatusConflict, "avatar_not_approved", "avatar is still processing or was not approved")
	case errors.Is(err, ErrProfileNotFound):
		writeError(w, http.StatusConflict, "profile_required", "create a profile before selecting an avatar")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeCreationTokenError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, locationproof.ErrRestrictedPlace):
		writeError(w, http.StatusForbidden, "restricted_place", "Place is restricted")
	case errors.Is(err, locationproof.ErrStaleProof):
		writeError(w, http.StatusBadRequest, "stale_location_proof", "Location proof is stale")
	case errors.Is(err, locationproof.ErrPoorAccuracy):
		writeError(w, http.StatusBadRequest, "poor_location_accuracy", "Location accuracy is insufficient")
	case errors.Is(err, locationproof.ErrOutsideBoundary):
		writeError(w, http.StatusForbidden, "outside_place_boundary", "Location is outside the Place boundary")
	case errors.Is(err, locationproof.ErrAmbiguousEligiblePlace):
		writeError(w, http.StatusConflict, "ambiguous_place", "Select one eligible Place")
	default:
		writeError(w, http.StatusBadRequest, "invalid_location_proof", "Invalid location proof")
	}
}

func writeEchoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidEcho):
		writeError(w, http.StatusBadRequest, "invalid_echo", "Invalid Echo")
	case errors.Is(err, ErrCreationProofMissing), errors.Is(err, ErrCreationProofUsed):
		writeError(w, http.StatusForbidden, "invalid_creation_token", "Creation token is invalid or already used")
	case errors.Is(err, ErrEchoConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "Idempotency key conflicts with an earlier request")
	case errors.Is(err, ErrEchoForbidden):
		writeError(w, http.StatusForbidden, "echo_forbidden", "Echo is not available to this user")
	case errors.Is(err, ErrEchoNotFound):
		writeError(w, http.StatusNotFound, "echo_not_found", "Echo not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseOptionalInt(value string, fallback int) (int, bool) {
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
