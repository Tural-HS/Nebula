// Russian strings for the colour theme and workspace accent (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tth = {
  theme: "Цветовой режим",
  themeHint: "Светлая, тёмная, бумага (мягче для глаз), полночь, высокий контраст или как в системе; применяется сразу.",
  themes: { system: "Система", light: "Светлая", dark: "Тёмная", green: "Зелёная", paper: "Бумага", midnight: "Полночь", contrast: "Высокий контраст" },
  accent: "Акцентный цвет",
  accentHint: "Кнопки, ссылки и выбранные элементы в этом рабочем пространстве используют этот цвет. Меняется для всех.",
  accentSaved: "Акцентный цвет сохранён",
  accents: { teal: "Бирюзовый (по умолчанию)", ocean: "Океан", indigo: "Индиго", violet: "Фиолетовый", plum: "Сливовый", forest: "Лесной", graphite: "Графит" },
} satisfies Messages["theme"];
