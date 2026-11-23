import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { BrowserRouter, Route, Routes } from "react-router";

import { ApiError, setUnauthorizedHandler } from "./api/client";
import { useAuth } from "./auth/context";
import { ToastProvider } from "./components/toast";
import { EmptyState, Spinner } from "./components/ui";
import { t } from "./i18n";
import { ItemsPage } from "./pages/ItemsPage";
import { ProjectsPage } from "./pages/ProjectsPage";
import { AuditLogPage } from "./pages/settings/AuditLogPage";
import { AutomationsPage } from "./pages/settings/AutomationsPage";
import { FieldsPage } from "./pages/settings/FieldsPage";
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
        // Client errors (403/404/422) won't fix themselves; only retry transient ones.
        retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
      },
    },
  });
}

export function App() {
  const { user, login } = useAuth();
  const [queryClient] = useState(makeQueryClient);

  useEffect(() => {
    setUnauthorizedHandler(() => void login());
  }, [login]);

  useEffect(() => {
    if (!user) void login();
  }, [user, login]);

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
