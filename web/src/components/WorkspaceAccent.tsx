import { useEffect } from "react";
import { matchPath, useLocation } from "react-router";

import { useWorkspace } from "../api/hooks";
import { applyAccent } from "../lib/accent";

/**
 * Applies the workspace accent (ADR-0035) while a /w/:ws page is open: the
 * brand-* CSS variables on <html> are overridden through the CSSOM (allowed
 * by the CSP, unlike style attributes in markup) and cleared elsewhere, so
 * the workspace list, forms and requests keep the Nebula colours.
 */
export function WorkspaceAccent() {
  const { pathname } = useLocation();
  const slug = matchPath({ path: "/w/:ws", end: false }, pathname)?.params.ws ?? "";
  return slug ? <Apply slug={slug} /> : <Clear />;
}

function Apply({ slug }: { slug: string }) {
  const accent = useWorkspace(slug).data?.accent;
  useEffect(() => {
    applyAccent(document.documentElement, accent);
  }, [accent]);
  useEffect(() => () => { applyAccent(document.documentElement, null); }, []);
  return null;
}

function Clear() {
  useEffect(() => { applyAccent(document.documentElement, null); }, []);
  return null;
}
