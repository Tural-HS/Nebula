// Russian strings for accepting an invitation (ADR-0016).
import type { Messages } from "../../i18n/locale";

export const ti = {
  title: "Приглашение в рабочее пространство",
  invitedBy: (who: string, ws: string, role: string) => `${who} приглашает вас в рабочее пространство «${ws}» с ролью «${role}».`,
  accept: "Принять",
  accepted: "Добро пожаловать!",
  alreadyMember: "Вы уже участник этого рабочего пространства.",
  open: "Открыть рабочее пространство",
  mismatch: (masked: string) => `Это приглашение для адреса ${masked}. Войдите с этим email.`,
  unverified: "Ваш email не подтверждён в системе идентификации, поэтому принять приглашение нельзя.",
  switchAccount: "Войти с другой учётной записью",
  invalid: "Приглашение не найдено: ссылка неверна, приглашение отозвано или отправлено новое.",
  used: "Это приглашение уже использовано.",
  expired: "Срок действия приглашения истёк. Попросите администратора отправить новое.",
} as const satisfies Messages["invite"];
