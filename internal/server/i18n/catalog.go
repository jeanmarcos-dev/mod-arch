package i18n

import "strings"

// Catalog maps lang -> code -> template. Each template supports {placeholder}
// substitutions through Interpolate. Adding a new code requires adding an
// entry to BOTH locales; the parity test guards against a one-sided edit.
var Catalog = map[string]map[string]string{
	"es": {
		CodeWSViewerReadonly: "modo espectador de solo lectura: el propietario es quien controla la sesión",
		CodeWSHostOnly:       "comando reservado al anfitrión de la sesión",
		CodeWSUnknownCommand: "comando desconocido: {cmd}",
		CodeWSJSONError:      "json: {err}",

		CodeAuthInvalidJSON:          "JSON inválido",
		CodeAuthValidationFailed:     "{reason}",
		CodeAuthAntibotFailed:        "verificación antibot falló",
		CodeAuthAntibotUnavailable:   "no fue posible verificar el desafío antibot",
		CodeAuthUserCreateFailed:     "no fue posible crear el usuario",
		CodeAuthNoSessionQuota:       "no hay cupo para sesiones nuevas",
		CodeAuthSMTPFailed:           "no se pudo enviar el correo de verificación. Intente de nuevo en unos minutos o contacte al administrador.",
		CodeAuthUnauthenticated:      "no autenticado",
		CodeAuthPersistenceDisabled:  "persistencia deshabilitada",
		CodeAuthInternalError:        "error interno",
		CodeAuthTokenRequired:        "token requerido",
		CodeAuthMagicInvalid:         "el enlace es inválido o expiró",
		CodeAuthMagicUserGone:        "el usuario asociado ya no existe",
		CodeAuthEmailNotAllowed:      "este correo no está autorizado para acceder a esta instancia",
		CodeAuthVerificationRequired: "esta cuenta requiere verificación: ingrese con el correo registrado",

		CodeViewerNotFound:      "vista no encontrada",
		CodeViewerFormInvalid:   "form inválido",
		CodeViewerEmptyPassword: "ingrese una contraseña",
		CodeViewerBadPassword:   "contraseña incorrecta",

		CodeAccessInviteRequired: "se requiere una invitación válida para acceder a esta instancia",
		CodeAccessInstanceClosed: "el acceso a esta instancia está cerrado",

		CodeRateLimitTooMany: "demasiados intentos, intente de nuevo en unos minutos",

		CodeAdminInvalidJSON:           "JSON inválido",
		CodeAdminInvalidAccessMode:     "valor inválido para access_mode",
		CodeAdminInvalidBool:           "valor inválido (true|false)",
		CodeAdminUnknownKey:            "clave desconocida",
		CodeAdminInviteMissingTarget:   "debe indicar correo o ID de estudiante",
		CodeAdminInviteInvalidRole:     "role inválido",
		CodeAdminInviteNothingToUpdate: "nada que actualizar",
		CodeAdminI18nUnsupportedLocale: "locale no soportado",

		CodeSimulatorCoresInvalidMin: "núcleos inválido ({cores}): debe ser >= 1",
		CodeSimulatorCoresInvalidMax: "núcleos inválido ({cores}): el máximo soportado es {max}",

		CodeFullTitleTab:      "Capacidad llena · MOD ARCH",
		CodeFullHeading:       "▸ Capacidad temporal alcanzada",
		CodeFullLeadStat:      "{full} / {max} sesiones",
		CodeFullLeadActive:    "Hay {full} sesiones activas en este servidor (cap configurado: {max}). Espere unos minutos y refresque, o únase a una sesión existente como espectador:",
		CodeFullSessionMeta:   "{viewers} viewers · creada hace {age} min",
		CodeFullSessionAction: "VER →",
		CodeFullEmpty:         "Por ahora ningún propietario activó el compartido, así que no hay sesiones públicas que visualizar. Refresque esta página cuando alguien lo haga, o intente más tarde para crear su propia sesión.",
		CodeFullFooter:        "◆ Demo SO · UCV",

		CodeAccessDeniedTitleTab: "Acceso restringido",
		CodeAccessDeniedHeading:  "ACCESO RESTRINGIDO",
		CodeAccessDeniedHint:     "Si dispone de una invitación, abra el enlace que le facilitó el docente.",

		CodeViewerPwTitleTab: "Acceso · MOD ARCH",
		CodeViewerPwHeading:  "VISTA RESTRINGIDA",
		CodeViewerPwLead:     "Esta sesión compartida requiere una contraseña para ser observada.",
		CodeViewerPwLabel:    "Contraseña",
		CodeViewerPwSubmit:   "INGRESAR",

		CodeViewInvalidTitleTab: "Enlace inválido",
		CodeViewInvalidHeading:  "▸ Enlace de espectador inválido",
		CodeViewInvalidLead:     "Este enlace no corresponde a ninguna sesión activa. Es posible que el propietario haya desactivado el compartido o cerrado la sesión.",
		CodeViewInvalidCreate:   "→ Crear una sesión nueva",

		CodePlaceholderHeading: "▸ MOD ARCH — backend listo",
		CodePlaceholderLead:    "El frontend Vue aún no está embebido.",
	},
	"en": {
		CodeWSViewerReadonly: "read-only spectator mode: the owner is the one controlling the session",
		CodeWSHostOnly:       "command reserved to the session host",
		CodeWSUnknownCommand: "unknown command: {cmd}",
		CodeWSJSONError:      "json: {err}",

		CodeAuthInvalidJSON:          "Invalid JSON",
		CodeAuthValidationFailed:     "{reason}",
		CodeAuthAntibotFailed:        "anti-bot verification failed",
		CodeAuthAntibotUnavailable:   "could not verify the anti-bot challenge",
		CodeAuthUserCreateFailed:     "could not create the user",
		CodeAuthNoSessionQuota:       "no quota for new sessions",
		CodeAuthSMTPFailed:           "could not send the verification email. Try again in a few minutes or contact the administrator.",
		CodeAuthUnauthenticated:      "not authenticated",
		CodeAuthPersistenceDisabled:  "persistence disabled",
		CodeAuthInternalError:        "internal error",
		CodeAuthTokenRequired:        "token required",
		CodeAuthMagicInvalid:         "the link is invalid or expired",
		CodeAuthMagicUserGone:        "the associated user no longer exists",
		CodeAuthEmailNotAllowed:      "this email is not authorized to access this instance",
		CodeAuthVerificationRequired: "this account requires verification: sign in with its registered email",

		CodeViewerNotFound:      "view not found",
		CodeViewerFormInvalid:   "invalid form",
		CodeViewerEmptyPassword: "enter a password",
		CodeViewerBadPassword:   "incorrect password",

		CodeAccessInviteRequired: "a valid invitation is required to access this instance",
		CodeAccessInstanceClosed: "access to this instance is closed",

		CodeRateLimitTooMany: "too many attempts, try again in a few minutes",

		CodeAdminInvalidJSON:           "Invalid JSON",
		CodeAdminInvalidAccessMode:     "invalid value for access_mode",
		CodeAdminInvalidBool:           "invalid value (true|false)",
		CodeAdminUnknownKey:            "unknown key",
		CodeAdminInviteMissingTarget:   "must provide email or student ID",
		CodeAdminInviteInvalidRole:     "invalid role",
		CodeAdminInviteNothingToUpdate: "nothing to update",
		CodeAdminI18nUnsupportedLocale: "unsupported locale",

		CodeSimulatorCoresInvalidMin: "invalid cores ({cores}): must be >= 1",
		CodeSimulatorCoresInvalidMax: "invalid cores ({cores}): the maximum supported is {max}",

		CodeFullTitleTab:      "Capacity full · MOD ARCH",
		CodeFullHeading:       "▸ Temporary capacity reached",
		CodeFullLeadStat:      "{full} / {max} sessions",
		CodeFullLeadActive:    "There are {full} active sessions on this server (configured cap: {max}). Wait a few minutes and refresh, or join an existing session as a spectator:",
		CodeFullSessionMeta:   "{viewers} viewers · created {age} min ago",
		CodeFullSessionAction: "VIEW →",
		CodeFullEmpty:         "No owner has turned sharing on yet, so there are no public sessions to watch. Refresh this page when someone does, or try again later to create your own session.",
		CodeFullFooter:        "◆ OS Demo · UCV",

		CodeAccessDeniedTitleTab: "Access restricted",
		CodeAccessDeniedHeading:  "ACCESS RESTRICTED",
		CodeAccessDeniedHint:     "If you have an invitation, open the link the teacher gave you.",

		CodeViewerPwTitleTab: "Access · MOD ARCH",
		CodeViewerPwHeading:  "RESTRICTED VIEW",
		CodeViewerPwLead:     "This shared session requires a password to be watched.",
		CodeViewerPwLabel:    "Password",
		CodeViewerPwSubmit:   "SIGN IN",

		CodeViewInvalidTitleTab: "Invalid link",
		CodeViewInvalidHeading:  "▸ Invalid spectator link",
		CodeViewInvalidLead:     "This link does not match any active session. The owner may have turned sharing off or closed the session.",
		CodeViewInvalidCreate:   "→ Create a new session",

		CodePlaceholderHeading: "▸ MOD ARCH — backend ready",
		CodePlaceholderLead:    "The Vue frontend is not embedded yet.",
	},
	"pt": {
		CodeWSViewerReadonly: "modo espectador somente leitura: o proprietário é quem controla a sessão",
		CodeWSHostOnly:       "comando reservado ao anfitrião da sessão",
		CodeWSUnknownCommand: "comando desconhecido: {cmd}",
		CodeWSJSONError:      "json: {err}",

		CodeAuthInvalidJSON:          "JSON inválido",
		CodeAuthValidationFailed:     "{reason}",
		CodeAuthAntibotFailed:        "a verificação antibot falhou",
		CodeAuthAntibotUnavailable:   "não foi possível verificar o desafio antibot",
		CodeAuthUserCreateFailed:     "não foi possível criar o usuário",
		CodeAuthNoSessionQuota:       "não há vagas para novas sessões",
		CodeAuthSMTPFailed:           "não foi possível enviar o e-mail de verificação. Tente novamente em alguns minutos ou entre em contato com o administrador.",
		CodeAuthUnauthenticated:      "não autenticado",
		CodeAuthPersistenceDisabled:  "persistência desabilitada",
		CodeAuthInternalError:        "erro interno",
		CodeAuthTokenRequired:        "token obrigatório",
		CodeAuthMagicInvalid:         "o link é inválido ou expirou",
		CodeAuthMagicUserGone:        "o usuário associado já não existe",
		CodeAuthEmailNotAllowed:      "este e-mail não está autorizado a acessar esta instância",
		CodeAuthVerificationRequired: "esta conta requer verificação: entre com o e-mail cadastrado",

		CodeViewerNotFound:      "visualização não encontrada",
		CodeViewerFormInvalid:   "formulário inválido",
		CodeViewerEmptyPassword: "informe uma senha",
		CodeViewerBadPassword:   "senha incorreta",

		CodeAccessInviteRequired: "é necessário um convite válido para acessar esta instância",
		CodeAccessInstanceClosed: "o acesso a esta instância está fechado",

		CodeRateLimitTooMany: "tentativas em excesso; tente novamente em alguns minutos",

		CodeAdminInvalidJSON:           "JSON inválido",
		CodeAdminInvalidAccessMode:     "valor inválido para access_mode",
		CodeAdminInvalidBool:           "valor inválido (true|false)",
		CodeAdminUnknownKey:            "chave desconhecida",
		CodeAdminInviteMissingTarget:   "informe o e-mail ou o ID do estudante",
		CodeAdminInviteInvalidRole:     "role inválido",
		CodeAdminInviteNothingToUpdate: "nada para atualizar",
		CodeAdminI18nUnsupportedLocale: "locale não suportado",

		CodeSimulatorCoresInvalidMin: "número de núcleos inválido ({cores}): precisa ser >= 1",
		CodeSimulatorCoresInvalidMax: "número de núcleos inválido ({cores}): o máximo suportado é {max}",

		CodeFullTitleTab:      "Capacidade lotada · MOD ARCH",
		CodeFullHeading:       "▸ Capacidade temporária atingida",
		CodeFullLeadStat:      "{full} / {max} sessões",
		CodeFullLeadActive:    "Há {full} sessões ativas neste servidor (limite configurado: {max}). Aguarde alguns minutos e atualize, ou entre em uma sessão existente como espectador:",
		CodeFullSessionMeta:   "{viewers} viewers · criada há {age} min",
		CodeFullSessionAction: "VER →",
		CodeFullEmpty:         "Por enquanto nenhum proprietário ativou o compartilhamento, portanto não há sessões públicas para acompanhar. Atualize esta página quando alguém ativar, ou tente mais tarde para criar a sua própria sessão.",
		CodeFullFooter:        "◆ Demo SO · UCV",

		CodeAccessDeniedTitleTab: "Acesso restrito",
		CodeAccessDeniedHeading:  "ACESSO RESTRITO",
		CodeAccessDeniedHint:     "Se você tiver um convite, abra o link que o(a) professor(a) forneceu.",

		CodeViewerPwTitleTab: "Acesso · MOD ARCH",
		CodeViewerPwHeading:  "VISUALIZAÇÃO RESTRITA",
		CodeViewerPwLead:     "Esta sessão compartilhada exige uma senha para ser acompanhada.",
		CodeViewerPwLabel:    "Senha",
		CodeViewerPwSubmit:   "ENTRAR",

		CodeViewInvalidTitleTab: "Link inválido",
		CodeViewInvalidHeading:  "▸ Link de espectador inválido",
		CodeViewInvalidLead:     "Este link não corresponde a nenhuma sessão ativa. É possível que o proprietário tenha desativado o compartilhamento ou encerrado a sessão.",
		CodeViewInvalidCreate:   "→ Criar uma nova sessão",

		CodePlaceholderHeading: "▸ MOD ARCH — backend pronto",
		CodePlaceholderLead:    "O frontend Vue ainda não está embutido.",
	},
}

// SupportedLocales is the closed set of locales we ship translations for.
// Anything else falls back to the default.
var SupportedLocales = []string{"es", "en", "pt"}

// DefaultLocale is the locale used when the request signals nothing else.
const DefaultLocale = "es"

// T returns the translated template for code in lang, with {placeholders}
// replaced from params. Unknown codes fall back to the code itself so a
// missing translation never produces an empty string at the client.
func T(lang, code string, params map[string]string) string {
	if _, ok := Catalog[lang]; !ok {
		lang = DefaultLocale
	}
	tpl, ok := Catalog[lang][code]
	if !ok {
		// Try the default locale before giving up.
		if tpl2, ok2 := Catalog[DefaultLocale][code]; ok2 {
			tpl = tpl2
		} else {
			return code
		}
	}
	return Interpolate(tpl, params)
}

// Interpolate replaces {key} occurrences in s with the matching value from
// params. Unknown placeholders are left untouched so the operator can spot
// them in logs.
func Interpolate(s string, params map[string]string) string {
	if len(params) == 0 {
		return s
	}
	for k, v := range params {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}
