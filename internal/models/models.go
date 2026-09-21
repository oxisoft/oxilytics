// Package models holds the domain types shared by store, services and httpapi.
package models

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

func (r Role) Valid() bool { return r == RoleAdmin || r == RoleViewer }

type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"`
	Role         Role       `json:"role"`
	TOTPSecret   *string    `json:"-"`
	TOTPRecovery *string    `json:"-"`
	TOTPEnabled  bool       `json:"totp_enabled"`
	Disabled     bool       `json:"disabled"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}

// APIToken is a credential for scripts and integrations. Owned by the user who
// created it and revoked automatically when that user is deleted. Plaintext is
// never stored: Hash is SHA-256, Prefix is the visible first characters so a
// token can be recognised in a list without exposing it.
//
// Tokens are read-only unless a capability was granted at creation. Capabilities
// are fixed at that moment and cannot be edited afterwards, so what a token can
// do is knowable from its creation record alone.
type APIToken struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	Hash       string     `json:"-"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	// CanRunSync permits POST /api/sync/runs and nothing else beyond reads.
	// The route still enforces the owner's run_sync permission.
	CanRunSync bool `json:"can_run_sync"`
	// Plaintext is populated only in the response that creates the token.
	Plaintext string `json:"token,omitempty"`
}

type Store string

const (
	StoreAppStore   Store = "appstore"
	StoreGooglePlay Store = "googleplay"
)

func (s Store) Valid() bool { return s == StoreAppStore || s == StoreGooglePlay }

type Platform string

const (
	PlatformIOS     Platform = "ios"
	PlatformMacOS   Platform = "macos"
	PlatformAndroid Platform = "android"
	PlatformWindows Platform = "windows"
)

type Product struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	IconURL     *string   `json:"icon_url"`
	Description *string   `json:"description"`
	Archived    bool      `json:"archived"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Apps        []App     `json:"apps,omitempty"`
}

// App is one store listing ("store app").
type App struct {
	ID                 int64      `json:"id"`
	Store              Store      `json:"store"`
	StoreAppID         string     `json:"store_app_id"`
	Name               string     `json:"name"`
	BundleID           *string    `json:"bundle_id"`
	Platform           Platform   `json:"platform"`
	IconURL            *string    `json:"icon_url"`
	ProductID          *int64     `json:"product_id"`
	SuggestedProductID *int64     `json:"suggested_product_id"`
	RatingAvg          *float64   `json:"rating_avg"`
	RatingCount        *int64     `json:"rating_count"`
	RatingUpdatedAt    *time.Time `json:"rating_updated_at"`
	IgnoredAt          *time.Time `json:"ignored_at,omitempty"`
	IgnoredBy          *int64     `json:"ignored_by,omitempty"`
	IgnoredReason      *string    `json:"ignored_reason,omitempty"`
	FirstSeenAt        time.Time  `json:"first_seen_at"`
	LastSyncedAt       *time.Time `json:"last_synced_at"`
}

// MetricDay is one (app, day, country) row. Country "*" is the total row.
type MetricDay struct {
	AppID           int64   `json:"app_id"`
	Day             string  `json:"day"` // YYYY-MM-DD
	Country         string  `json:"country"`
	Downloads       int64   `json:"downloads"`
	Redownloads     int64   `json:"redownloads"`
	Updates         int64   `json:"updates"`
	Uninstalls      int64   `json:"uninstalls"`
	ActiveDevices   *int64  `json:"active_devices"`
	Crashes         int64   `json:"crashes"`
	ANRs            int64   `json:"anrs"`
	SourceUpdatedAt *string `json:"source_updated_at"`
}

type Review struct {
	ID                 int64      `json:"id"`
	AppID              int64      `json:"app_id"`
	StoreReviewID      string     `json:"store_review_id"`
	Rating             int        `json:"rating"`
	Title              *string    `json:"title"`
	Body               *string    `json:"body"`
	Author             *string    `json:"author"`
	Country            *string    `json:"country"`
	Language           *string    `json:"language"`
	AppVersion         *string    `json:"app_version"`
	Device             *string    `json:"device"`
	CreatedAt          time.Time  `json:"created_at"`
	EditedAt           *time.Time `json:"edited_at"`
	DeveloperReply     *string    `json:"developer_reply"`
	DeveloperRepliedAt *time.Time `json:"developer_replied_at"`
	FetchedAt          time.Time  `json:"fetched_at"`
	// joined
	AppName  string   `json:"app_name,omitempty"`
	Store    Store    `json:"store,omitempty"`
	Platform Platform `json:"platform,omitempty"`
	ProductID *int64  `json:"product_id,omitempty"`
}

type SyncMode string

const (
	SyncFull  SyncMode = "full"
	SyncDelta SyncMode = "delta"
)

func (m SyncMode) Valid() bool { return m == SyncFull || m == SyncDelta }

type SyncTrigger string

const (
	TriggerManual   SyncTrigger = "manual"
	TriggerSchedule SyncTrigger = "schedule"
)

type SyncStatus string

const (
	SyncQueued      SyncStatus = "queued"
	SyncRunning     SyncStatus = "running"
	SyncSucceeded   SyncStatus = "succeeded"
	SyncFailed      SyncStatus = "failed"
	SyncCancelled   SyncStatus = "cancelled"
	SyncInterrupted SyncStatus = "interrupted"
)

type SyncRun struct {
	ID          int64       `json:"id"`
	Store       Store       `json:"store"`
	Mode        SyncMode    `json:"mode"`
	Trigger     SyncTrigger `json:"trigger"`
	RequestedBy *int64      `json:"requested_by"`
	Status      SyncStatus  `json:"status"`
	StartedAt   *time.Time  `json:"started_at"`
	FinishedAt  *time.Time  `json:"finished_at"`
	RangeFrom   *string     `json:"range_from"`
	RangeTo     *string     `json:"range_to"`
	AppsTotal   int         `json:"apps_total"`
	AppsDone    int         `json:"apps_done"`
	RowsMetrics int64       `json:"rows_metrics"`
	RowsReviews int64       `json:"rows_reviews"`
	Error       *string     `json:"error"`
	Stats       *string     `json:"stats"` // JSON
	CreatedAt   time.Time   `json:"created_at"`
	// joined
	RequestedByName *string `json:"requested_by_name,omitempty"`
}

type SyncRunLog struct {
	ID      int64     `json:"id"`
	RunID   int64     `json:"run_id"`
	TS      time.Time `json:"ts"`
	Level   string    `json:"level"`
	AppID   *int64    `json:"app_id"`
	Message string    `json:"message"`
}

type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
