import { ApiError } from "../../api/client";
import { useToast } from "../../components/toast-context";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";

/** Callbacks for admin mutations: success toast, readable error toast. */
export function useFeedback() {
  const toast = useToast();
  return {
    ok: (text: string = ta.saved) => () => {
      toast("success", text);
    },
    fail: (err: unknown) => {
      if (err instanceof ApiError) toast("error", err.field ? `${err.field}: ${err.message}` : err.message);
      else toast("error", t.errorGeneric);
    },
  };
}
