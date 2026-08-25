/**
 * Joins class names, dropping anything falsy.
 *
 * CSS-module lookups type as `string | undefined` under
 * `noUncheckedIndexedAccess`, which most DOM props accept but some library
 * props do not. Rather than weaken the compiler setting for the whole app,
 * class names go through here and come out a plain string.
 */
export function cx(...parts: readonly (string | false | undefined | null)[]): string {
  return parts.filter(Boolean).join(' ')
}
