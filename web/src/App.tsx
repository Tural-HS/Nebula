import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { lazy, Suspense, useEffect, useState, type ComponentType } from "react";
import { BrowserRouter, Route, Routes } from "react-router";

import { setUnauthorizedHandler } from "./api/client";
import { useAuth } from "./auth/context";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ToastProvider } from "./components/toast";
import { retryDelay, shouldRetry, shouldRetryMutation } from "./lib/retry";
import { EmptyState, Spinner } from "./components/ui";
import { WorkspaceAccent } from "./components/WorkspaceAccent";
import { t } from "./i18n";
import { ItemsPage } from "./pages/ItemsPage";
import { AbsencePage } from "./pages/AbsencePage";
import { ApprovalsInboxPage } from "./pages/ApprovalsInboxPage";
import { ReportsPage } from "./pages/ReportsPage";
import { ProjectsPage } from "./pages/ProjectsPage";
import { HomePage } from "./pages/HomePage";
import { ProjectGate } from "./pages/ProjectGate";

// Pages that are not needed for the first paint load on demand, each as its
// own chunk; the workspace, project and item pages stay in the main bundle.
const named = <K extends string>(load: () => Promise<Record<K, ComponentType>>, name: K) =>
  lazy(async () => ({ default: (await load())[name] }));
const ImportPage = named(() => import("./pages/ImportPage"), "ImportPage");
const RecurringPage = named(() => import("./pages/RecurringPage"), "RecurringPage");
const ProjectSettingsPage = named(() => import("./pages/ProjectSettingsPage"), "ProjectSettingsPage");
const InvitePage = named(() => import("./pages/InvitePage"), "InvitePage");
const FormPage = named(() => import("./pages/FormPage"), "FormPage");
const FormsPage = named(() => import("./pages/FormsPage"), "FormsPage");
const MyRequestsPage = named(() => import("./pages/MyRequestsPage"), "MyRequestsPage");
const MyRequestPage = named(() => import("./pages/MyRequestsPage"), "MyRequestPage");
const AuditLogPage = named(() => import("./pages/settings/AuditLogPage"), "AuditLogPage");
const AutomationsPage = named(() => import("./pages/settings/AutomationsPage"), "AutomationsPage");
const EmailPage = named(() => import("./pages/settings/EmailPage"), "EmailPage");
const FieldsPage = named(() => import("./pages/settings/FieldsPage"), "FieldsPage");
const GeneralPage = named(() => import("./pages/settings/GeneralPage"), "GeneralPage");
const ItemTypesPage = named(() => import("./pages/settings/ItemTypesPage"), "ItemTypesPage");
const MembersPage = named(() => import("./pages/settings/MembersPage"), "MembersPage");
const WebhooksPage = named(() => import("./pages/settings/WebhooksPage"), "WebhooksPage");
const WorkflowsPage = named(() => import("./pages/settings/WorkflowsPage"), "WorkflowsPage");
const WorkCalendarPage = named(() => import("./pages/settings/WorkCalendarPage"), "WorkCalendarPage");

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
          <WorkspaceAccent />
          <ErrorBoundary>
          <Suspense fallback={<Spinner />}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/w/:ws" element={<ProjectsPage />} />
            <Route path="/w/:ws/approvals" element={<ApprovalsInboxPage />} />
            <Route path="/w/:ws/absence" element={<AbsencePage />} />
            <Route path="/w/:ws/reports" element={<ReportsPage />} />
            <Route path="/w/:ws/p/:project" element={<ProjectGate><ItemsPage /></ProjectGate>} />
            <Route path="/w/:ws/p/:project/import" element={<ProjectGate><ImportPage /></ProjectGate>} />
            <Route path="/w/:ws/p/:project/recurring" element={<ProjectGate><RecurringPage /></ProjectGate>} />
            <Route path="/w/:ws/p/:project/settings" element={<ProjectGate><ProjectSettingsPage /></ProjectGate>} />
            <Route path="/invite/:token" element={<InvitePage />} />
            <Route path="/f/:ref" element={<FormPage />} />
            <Route path="/requests" element={<MyRequestsPage />} />
            <Route path="/requests/:id" element={<MyRequestPage />} />
            <Route path="/w/:ws/p/:project/forms" element={<ProjectGate><FormsPage /></ProjectGate>} />
            <Route path="/w/:ws/settings" element={<MembersPage />} />
            <Route path="/w/:ws/settings/general" element={<GeneralPage />} />
            <Route path="/w/:ws/settings/calendar" element={<WorkCalendarPage />} />
            <Route path="/w/:ws/settings/members" element={<MembersPage />} />
            <Route path="/w/:ws/settings/fields" element={<FieldsPage />} />
            <Route path="/w/:ws/settings/item-types" element={<ItemTypesPage />} />
            <Route path="/w/:ws/settings/workflows" element={<WorkflowsPage />} />
            <Route path="/w/:ws/settings/email" element={<EmailPage />} />
            <Route path="/w/:ws/settings/webhooks" element={<WebhooksPage />} />
            <Route path="/w/:ws/settings/automations" element={<AutomationsPage />} />
            <Route path="/w/:ws/settings/audit" element={<AuditLogPage />} />
            <Route path="*" element={<EmptyState>{t.notFound}</EmptyState>} />
          </Routes>
          </Suspense>
          </ErrorBoundary>
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}
