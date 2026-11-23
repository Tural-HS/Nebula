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
            <Route path="*" element={<EmptyState>{t.notFound}</EmptyState>} />
          </Routes>
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}
