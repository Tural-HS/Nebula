// Plural forms for dictionaries whose words change with the count: English
// (one/other) and Russian (one/few/many). Azerbaijani and Turkish do not
// inflect nouns after numbers.
type Forms = Partial<Record<Intl.LDMLPluralRule, string>> & { other: string };

const rules = new Map<string, Intl.PluralRules>();

/** plural("ru", 3, { one: "задача", few: "задачи", many: "задач", other: "задачи" }) → "задачи". */
export function plural(locale: string, n: number, forms: Forms): string {
  let r = rules.get(locale);
  if (!r) {
    r = new Intl.PluralRules(locale);
    rules.set(locale, r);
  }
  return forms[r.select(n)] ?? forms.other;
}
