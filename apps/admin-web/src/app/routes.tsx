import type { QueryClient } from "@tanstack/react-query";
import { type RouteObject, redirect } from "react-router";

import { requireAdminLoader } from "../auth/requireAdmin";
import { AdminsPage } from "../features/admins/AdminsPage";
import { AuditPage } from "../features/audit/AuditPage";
import { CustomerDetailPage } from "../features/customers/CustomerDetailPage";
import { CustomersPage } from "../features/customers/CustomersPage";
import { ErrorPage } from "../features/error/ErrorPage";
import { LoginPage } from "../features/login/LoginPage";
import { NoAccessPage } from "../features/login/NoAccessPage";
import { RegistrationDisabledPage } from "../features/login/RegistrationDisabledPage";
import { RecoveryPage } from "../features/recovery/RecoveryPage";
import { SettingsPage } from "../features/settings/SettingsPage";
import { VerificationPage } from "../features/verification/VerificationPage";
import { AuthLayout } from "./AuthLayout";
import { ConsoleLayout } from "./ConsoleLayout";
import { Spinner } from "../shared/ui/Spinner";
import { Root } from "./Root";
import { RouteError } from "./RouteError";

export function createRoutes(queryClient: QueryClient): RouteObject[] {
  return [
    {
      element: <Root />,
      errorElement: <RouteError />,
      children: [
        {
          element: <AuthLayout />,
          children: [
            { path: "/login", element: <LoginPage /> },
            { path: "/recovery", element: <RecoveryPage /> },
            { path: "/settings", element: <SettingsPage /> },
            { path: "/verification", element: <VerificationPage /> },
            { path: "/error", element: <ErrorPage /> },
            // Kratos' registration ui_url: intentionally renders no form.
            { path: "/registration", element: <RegistrationDisabledPage /> },
            { path: "/no-access", element: <NoAccessPage /> },
          ],
        },
        {
          id: "console",
          loader: requireAdminLoader(queryClient),
          element: <ConsoleLayout />,
          hydrateFallbackElement: <Spinner />,
          errorElement: <RouteError />,
          children: [
            { path: "/", loader: () => redirect("/customers") },
            { path: "/customers", element: <CustomersPage /> },
            { path: "/customers/:id", element: <CustomerDetailPage /> },
            { path: "/admins", element: <AdminsPage /> },
            { path: "/audit", element: <AuditPage /> },
          ],
        },
        { path: "*", loader: () => redirect("/") },
      ],
    },
  ];
}
