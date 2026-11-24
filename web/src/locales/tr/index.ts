// Turkish dictionary (ADR-0025); mirrors the Azerbaijani source of truth.
import type { Messages } from "../../i18n/locale";
import { ta as admin } from "./admin";
import { ta as attach } from "./attach";
import { tc as calendar } from "./calendar";
import { t as core } from "./core";
import { ti as importer } from "./import";
import { ti as invite } from "./invite";
import { tm as mobile } from "./mobile";
import { tn as notifications } from "./notifications";
import { tp as project } from "./project";
import { tr as recur } from "./recur";
import { ts as search } from "./search";
import { ts as subtasks } from "./subtasks";
import { tt as templates } from "./templates";
import { tv as views } from "./views";
import { tw as workdays } from "./workdays";

export const tr: Messages = { admin, attach, calendar, core, importer, invite, mobile, notifications, project, recur, search, subtasks, templates, views, workdays };
