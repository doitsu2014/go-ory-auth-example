import { Outlet } from "react-router";

import { Toaster } from "../shared/ui/Toaster";

export function Root() {
  return (
    <>
      <Outlet />
      <Toaster />
    </>
  );
}
