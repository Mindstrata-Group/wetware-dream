"use client";

import Link from "next/link";
import dynamic from "next/dynamic";
import { BrandLogo } from "@/components/BrandLogo";
import { AdminPageContext } from "../components/AdminPageContext";
import { ErrorDebugDetails } from "../components/ErrorDebugDetails";
import { TAB_ICONS, mobileTabMeta } from "../constants";
import type { useAdminPageController } from "./useAdminPageController";

const adminTabLoading = () => <div className="ms-info-box">Загрузка раздела…</div>;
const AdminProfileTab = dynamic(() => import("../components/AdminProfileTab").then((m) => m.AdminProfileTab), { loading: adminTabLoading });
const AdminSystemTab = dynamic(() => import("../components/AdminSystemTab").then((m) => m.AdminSystemTab), { loading: adminTabLoading });
const AdminStatsTab = dynamic(() => import("../components/AdminStatsTab").then((m) => m.AdminStatsTab), { loading: adminTabLoading });
const AdminUsersTab = dynamic(() => import("../components/AdminUsersTab").then((m) => m.AdminUsersTab), { loading: adminTabLoading });
const AdminAccessTab = dynamic(() => import("../components/AdminAccessTab").then((m) => m.AdminAccessTab), { loading: adminTabLoading });
const AdminModesTab = dynamic(() => import("../components/AdminModesTab").then((m) => m.AdminModesTab), { loading: adminTabLoading });
const AdminTariffsTab = dynamic(() => import("../components/AdminTariffsTab").then((m) => m.AdminTariffsTab), { loading: adminTabLoading });
const AdminPromocodesTab = dynamic(() => import("../components/AdminPromocodesTab").then((m) => m.AdminPromocodesTab), { loading: adminTabLoading });
const AdminExportsTab = dynamic(() => import("../components/AdminExportsTab").then((m) => m.AdminExportsTab), { loading: adminTabLoading });
const AdminOrchestrationTab = dynamic(() => import("../components/AdminOrchestrationTab").then((m) => m.AdminOrchestrationTab), { loading: adminTabLoading });
const AdminBroadcastTab = dynamic(() => import("../components/AdminBroadcastTab").then((m) => m.AdminBroadcastTab), { loading: adminTabLoading });
const AdminPaymentsTab = dynamic(() => import("../components/AdminPaymentsTab").then((m) => m.AdminPaymentsTab), { loading: adminTabLoading });
const AdminNotificationsTab = dynamic(() => import("../components/AdminNotificationsTab").then((m) => m.AdminNotificationsTab), { loading: adminTabLoading });
const AdminContentTab = dynamic(() => import("../components/AdminContentTab").then((m) => m.AdminContentTab), { loading: adminTabLoading });
const AdminBlogCmsTab = dynamic(() => import("../components/AdminBlogCmsTab").then((m) => m.AdminBlogCmsTab), { loading: adminTabLoading });

export function AdminPageShell({ controller }: { controller: ReturnType<typeof useAdminPageController> }) {
  const { adminPageContext, headerMobile, visibleTabs, tab, selectTab, error, errorDebug, notice, logout } = controller;

  return (
    <div className="ms-page ms-admin-page">
      <header style={{
        height: headerMobile ? 56 : 64,
        background: "var(--card)",
        borderBottom: "1px solid var(--line)",
        display: "flex",
        alignItems: "center",
        gap: 8,
        padding: `0 ${headerMobile ? 14 : 20}px`,
        flexShrink: 0,
        position: "sticky",
        top: 0,
        zIndex: 20,
      }}>
        <BrandLogo priority />
        <nav style={{
          marginLeft: "auto",
          display: "flex",
          alignItems: "center",
          gap: headerMobile ? 6 : 10,
          flexShrink: 0,
          flexWrap: "nowrap",
        }}>
          <Link href="/chat" style={{
            display: "inline-flex",
            alignItems: "center",
            height: headerMobile ? 34 : 38,
            padding: `0 ${headerMobile ? 12 : 16}px`,
            borderRadius: 10,
            background: "#1D9E75",
            color: "#fff",
            fontSize: 13,
            fontWeight: 600,
            textDecoration: "none",
            whiteSpace: "nowrap",
            flexShrink: 0,
          }}>
            В чат
          </Link>
          <button className="ms-admin-header-button" onClick={logout} style={{
            display: "inline-flex",
            alignItems: "center",
            height: headerMobile ? 34 : 38,
            padding: `0 ${headerMobile ? 12 : 16}px`,
            borderRadius: 10,
            border: "1px solid var(--line)",
            background: "var(--card)",
            color: "var(--foreground)",
            fontSize: 13,
            fontWeight: 500,
            cursor: "pointer",
            whiteSpace: "nowrap",
            fontFamily: "inherit",
            flexShrink: 0,
          }}>
            Выйти
          </button>
        </nav>
      </header>

      {headerMobile ? (
        <nav className="ms-admin-tabs-strip" aria-label="Разделы">
          {visibleTabs.map((t) => (
            <button
              key={t.id + "-strip"}
              type="button"
              className={`ms-admin-tabs-strip-button${tab === t.id ? " is-active" : ""}`}
              onClick={() => selectTab(t.id)}
            >
              {TAB_ICONS[t.id]} {mobileTabMeta[t.id].label}
            </button>
          ))}
        </nav>
      ) : null}

      <main className="container ms-admin-container">
        {error ? (
          <div className="ms-error-box ms-admin-message" style={{ marginTop: 12 }}>
            <span>{error}</span>
            <ErrorDebugDetails debug={errorDebug} />
          </div>
        ) : notice ? (
          <div className="ms-success-box ms-admin-message" style={{ marginTop: 12 }}>{notice}</div>
        ) : null}

        <div className="ms-admin-layout" style={{ marginTop: 8 }}>
          <aside className="ms-admin-tabs">
            {visibleTabs.map((t) => (
              <button
                key={t.id}
                className={`ms-admin-tab-button${tab === t.id ? " is-active" : ""}`}
                onClick={() => selectTab(t.id)}
              >
                <span style={{ color: tab === t.id ? "var(--accent-strong)" : "var(--muted)", flexShrink: 0, display: "inline-flex" }}>{TAB_ICONS[t.id]}</span>{t.label}
              </button>
            ))}
          </aside>

          <section className="ms-admin-content">
            <AdminPageContext.Provider value={adminPageContext}>
              {tab === "profile" && <AdminProfileTab />}
              {tab === "system" && <AdminSystemTab />}
              {tab === "stats" && <AdminStatsTab />}
              {tab === "users" && <AdminUsersTab />}
              {tab === "access" && <AdminAccessTab />}
              {tab === "modes" && <AdminModesTab />}
              {tab === "tariffs" && <AdminTariffsTab />}
              {tab === "promocodes" && <AdminPromocodesTab />}
              {tab === "exports" && <AdminExportsTab />}
              {tab === "orchestration" && <AdminOrchestrationTab />}
              {tab === "broadcast" && <AdminBroadcastTab />}
              {tab === "payments" && <AdminPaymentsTab />}
              {tab === "notifications" && <AdminNotificationsTab />}
              {tab === "content" && <AdminContentTab />}
              {tab === "blog" && <AdminBlogCmsTab />}
            </AdminPageContext.Provider>
          </section>
        </div>
      </main>
    </div>
  );
}
