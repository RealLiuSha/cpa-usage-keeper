// The product default language is zh (DEFAULT_LANGUAGE in ./index.ts), but the
// suite's UI-copy assertions are written against the English resources. Pin en
// once here so no test inherits the product default: a change to what users see
// on first visit must not decide what 1000+ assertions compare against.
import i18n from './index';

await i18n.changeLanguage('en');
