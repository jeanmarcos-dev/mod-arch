// Package i18n holds the backend-side localization catalog plus helpers for
// resolving a lang per request and emitting translated error payloads. The
// frontend mirrors every code under backend.errors.<code> in its own catalogs,
// so a missing entry on either side surfaces in the runtime parity test.
package i18n

// Error codes shipped to clients in {"error_code": "..."} responses.
const (
	CodeWSViewerReadonly = "ws.viewer_readonly"
	CodeWSHostOnly       = "ws.host_only"
	CodeWSUnknownCommand = "ws.unknown_command"
	CodeWSJSONError      = "ws.json_error"

	CodeAuthInvalidJSON         = "auth.invalid_json"
	CodeAuthValidationFailed    = "auth.validation_failed"
	CodeAuthAntibotFailed       = "auth.antibot_failed"
	CodeAuthAntibotUnavailable  = "auth.antibot_unavailable"
	CodeAuthUserCreateFailed    = "auth.user_create_failed"
	CodeAuthNoSessionQuota      = "auth.no_session_quota"
	CodeAuthSMTPFailed          = "auth.smtp_failed"
	CodeAuthUnauthenticated     = "auth.unauthenticated"
	CodeAuthPersistenceDisabled = "auth.persistence_disabled"
	CodeAuthInternalError       = "auth.internal_error"
	CodeAuthTokenRequired       = "auth.token_required"
	CodeAuthMagicInvalid        = "auth.magic_invalid"
	CodeAuthMagicUserGone       = "auth.magic_user_gone"

	CodeViewerNotFound      = "viewer.not_found"
	CodeViewerFormInvalid   = "viewer.form_invalid"
	CodeViewerEmptyPassword = "viewer.empty_password"
	CodeViewerBadPassword   = "viewer.bad_password"

	CodeAccessInviteRequired = "access.invite_required"
	CodeAccessInstanceClosed = "access.instance_closed"

	CodeRateLimitTooMany = "ratelimit.too_many"

	CodeAdminInvalidJSON            = "admin.invalid_json"
	CodeAdminInvalidAccessMode      = "admin.invalid_access_mode"
	CodeAdminInvalidBool            = "admin.invalid_bool"
	CodeAdminUnknownKey             = "admin.unknown_key"
	CodeAdminInviteMissingTarget    = "admin.invite_missing_target"
	CodeAdminInviteInvalidRole      = "admin.invite_invalid_role"
	CodeAdminInviteNothingToUpdate  = "admin.invite_nothing_to_update"
	CodeAdminI18nUnsupportedLocale  = "i18n.unsupported_locale"

	CodeSimulatorCoresInvalidMin = "simulator.cores_invalid_min"
	CodeSimulatorCoresInvalidMax = "simulator.cores_invalid_max"

	// Codes used exclusively by server-rendered HTML templates.
	CodeFullTitleTab      = "full.title_tab"
	CodeFullHeading       = "full.heading"
	CodeFullLeadStat      = "full.lead_stat"
	CodeFullLeadActive    = "full.lead_active"
	CodeFullSessionMeta   = "full.session_meta"
	CodeFullSessionAction = "full.session_action"
	CodeFullEmpty         = "full.empty"
	CodeFullFooter        = "full.footer"

	CodeAccessDeniedTitleTab = "access_denied.title_tab"
	CodeAccessDeniedHeading  = "access_denied.heading"
	CodeAccessDeniedHint     = "access_denied.hint"

	CodeViewerPwTitleTab = "viewer_password.title_tab"
	CodeViewerPwHeading  = "viewer_password.heading"
	CodeViewerPwLead     = "viewer_password.lead"
	CodeViewerPwLabel    = "viewer_password.label"
	CodeViewerPwSubmit   = "viewer_password.submit"

	CodeViewInvalidTitleTab = "view_invalid.title_tab"
	CodeViewInvalidHeading  = "view_invalid.heading"
	CodeViewInvalidLead     = "view_invalid.lead"
	CodeViewInvalidCreate   = "view_invalid.create_new"

	CodePlaceholderHeading = "placeholder.heading"
	CodePlaceholderLead    = "placeholder.lead"
)
