// English strings for workspace/project templates and the Mattermost
// Boards import (ADR-0020).
import type { Messages } from "../../i18n/locale";

export const tt = {
  template: "Starter template",
  templateHint: "Statuses and item types can be changed later in Settings.",

  projectTemplate: "Template project",
  projectTemplateNone: "None (empty project)",
  projectTemplateHint: "The following are copied from the selected project. Items are not copied.",
  copyMembers: "Project members",
  copyRecurrences: "Recurring items",
  copyViews: "Shared views",
} satisfies Messages["templates"];
