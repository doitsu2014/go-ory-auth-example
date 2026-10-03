import { QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { RouterProvider, createBrowserRouter } from "react-router";

import { createQueryClient } from "./queryClient";
import { createRoutes } from "./routes";

function createApp() {
  let navigate: (path: string) => void = (path) => window.location.assign(path);
  const queryClient = createQueryClient({
    navigate: (path) => navigate(path),
    currentPath: () => `${window.location.pathname}${window.location.search}`,
  });
  const router = createBrowserRouter(createRoutes(queryClient));
  navigate = (path) => void router.navigate(path);
  return { queryClient, router };
}

export function App() {
  const [{ queryClient, router }] = useState(createApp);
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
