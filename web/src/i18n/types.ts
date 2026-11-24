// The Azerbaijani dictionary is the source of truth; every other language must
// have the same keys, array lengths and function parameters (checked by tsc).

/** Literal string types become string, so translations may differ in text. */
export type Widen<T> = T extends string
  ? string
  : T extends (...args: infer A) => infer R
    ? (...args: A) => Widen<R>
    : T extends readonly unknown[]
      ? { readonly [K in keyof T]: Widen<T[K]> }
      : T extends object
        ? { readonly [K in keyof T]: Widen<T[K]> }
        : T;
