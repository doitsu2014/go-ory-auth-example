import { QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { RouterProvider, createMemoryRouter } from "react-router";

import { createQueryClient } from "../app/queryClient";
import { createRoutes } from "../app/routes";

/** Renders the real route tree at `path` with a fresh QueryClient. */
export function renderApp(path: string) {
  let navigate: (p: string) => void = () => undefined;
  const queryClient = createQueryClient({
    navigate: (p) => navigate(p),
    currentPath: () => `${router.state.location.pathname}${router.state.location.search}`,
  });
  const router = createMemoryRouter(createRoutes(queryClient), { initialEntries: [path] });
  navigate = (p) => void router.navigate(p);
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...utils, router, queryClient };
}
