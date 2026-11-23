import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { BrowserRouter, Route, Routes } from "react-router";

import { setUnauthorizedHandler } from "./api/client";
import { useAuth } from "./auth/context";
import { ToastProvider } from "./components/toast";
import { retryDelay, shouldRetry, shouldRetryMutation } from "./lib/retry";
import { EmptyState, Spinner } from "./components/ui";
import { t } from "./i18n";
import { ItemsPage } from "./pages/ItemsPage";
import { ProjectsPage } from "./pages/ProjectsPage";
import { AuditLogPage } from "./pages/settings/AuditLogPage";
import { AutomationsPage } from "./pages/settings/AutomationsPage";
import { FieldsPage } from "./pages/settings/FieldsPage";
import { GeneralPage } from "./pages/settings/GeneralPage";
import { ItemTypesPage } from "./pages/settings/ItemTypesPage";
import { MembersPage } from "./pages/settings/MembersPage";
import { WebhooksPage } from "./pages/settings/WebhooksPage";
import { WorkflowsPage } from "./pages/settings/WorkflowsPage";
import { WorkspacesPage } from "./pages/WorkspacesPage";

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        refetchOnWindowFocus: true,
        // Only transient failures (429, 5xx, network) are retried; see lib/retry.
        retry: shouldRetry,
        retryDelay,
      },
      mutations: {
        // Only 429: the request was rejected before it ran (lib/retry).
        retry: shouldRetryMutation,
        retryDelay,
      },
    },
  });
}

export function App() {
  const { user, login, signingOut } = useAuth();
  const [queryClient] = useState(makeQueryClient);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      if (!signingOut) void login();
    });
  }, [login, signingOut]);

  useEffect(() => {
    if (!user && !signingOut) void login();
  }, [user, login, signingOut]);

  if (!user) return <Spinner label={t.signingIn} />;

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<WorkspacesPage />} />
            <Route path="/w/:ws" element={<ProjectsPage />} />
            <Route path="/w/:ws/p/:project" element={<ItemsPage />} />
            <Route path="/w/:ws/settings" element={<MembersPage />} />
            <Route path="/w/:ws/settings/general" element={<GeneralPage />} />
            <Route path="/w/:ws/settings/members" element={<MembersPage />} />
            <Route path="/w/:ws/settings/fields" element={<FieldsPage />} />
            <Route path="/w/:ws/settings/item-types" element={<ItemTypesPage />} />
            <Route path="/w/:ws/settings/workflows" element={<WorkflowsPage />} />
            <Route path="/w/:ws/settings/webhooks" element={<WebhooksPage />} />
            <Route path="/w/:ws/settings/automations" element={<AutomationsPage />} />
            <Route path="/w/:ws/settings/audit" element={<AuditLogPage />} />
            <Route path="*" element={<EmptyState>{t.notFound}</EmptyState>} />
          </Routes>
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}
