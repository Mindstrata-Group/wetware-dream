"use client";

import { AdminPageShell } from "./_hooks/AdminPageShell";
import { useAdminPageController } from "./_hooks/useAdminPageController";

export default function AdminPage() {
  const controller = useAdminPageController();
  return <AdminPageShell controller={controller} />;
}
