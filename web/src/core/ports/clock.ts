/** Time, injected so anything that reasons about "now" stays testable. */
export interface Clock {
  now(): Date
}
