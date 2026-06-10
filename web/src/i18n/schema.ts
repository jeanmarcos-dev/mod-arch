// Single source of truth for the i18n message shape. en.ts uses `satisfies
// MessageSchema` to force compile-time parity, and the DefineLocaleMessage
// module augmentation in index.ts wires this type into useI18n() so $t and t
// autocomplete the available keys.

import type es from './locales/es';

export type MessageSchema = typeof es;

export type Locale = 'es' | 'en' | 'pt';

export const SUPPORTED_LOCALES: Locale[] = ['es', 'en', 'pt'];

export const DEFAULT_LOCALE: Locale = 'es';
