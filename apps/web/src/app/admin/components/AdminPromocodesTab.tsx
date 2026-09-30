"use client";

import { PromoCreateForm } from "./_promos/PromoCreateForm";
import { PromoCreateResult } from "./_promos/PromoCreateResult";
import { PromoListSection } from "./_promos/PromoListSection";

export function AdminPromocodesTab() {
  return (
    <div className="ms-admin-stack">
      <PromoCreateForm />
      <PromoCreateResult />
      <PromoListSection />
    </div>
  );
}
