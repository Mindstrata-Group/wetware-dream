"use client";

import { UserCreateForm } from "./_users/UserCreateForm";
import { UserListSection } from "./_users/UserListSection";

export function AdminUsersTab() {
  return (
    <div className="ms-admin-stack">
      <UserCreateForm />
      <UserListSection />
    </div>
  );
}
