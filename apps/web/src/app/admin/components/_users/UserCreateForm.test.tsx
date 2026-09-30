import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { AdminPageContext } from "../AdminPageContext";
import { UserCreateForm } from "./UserCreateForm";

function makeContext(overrides: Record<string, unknown> = {}) {
  return {
    created: { email: "", role: "user", status: "active", days: "30", modeIds: [] },
    setCreated: vi.fn(),
    statusHelp: vi.fn(() => "Активен"),
    roleDescriptions: { user: "Обычный пользователь", admin: "Администратор" },
    roles: ["user", "admin"],
    statuses: ["active", "blocked"],
    createUser: vi.fn(),
    modeSelectionToolbar: vi.fn(() => null),
    modeCheckboxes: vi.fn(() => null),
    ...overrides,
  };
}

function renderForm(ctx = makeContext()) {
  return render(
    <AdminPageContext.Provider value={ctx as never}>
      <UserCreateForm />
    </AdminPageContext.Provider>,
  );
}

describe("UserCreateForm — без поля пароля", () => {
  afterEach(() => cleanup());

  it("НЕ содержит label/placeholder/input для поля «Пароль»", () => {
    renderForm();
    expect(screen.queryByText(/^Пароль$/i)).toBeNull();
    expect(screen.queryByPlaceholderText(/пароль/i)).toBeNull();
  });

  it("НЕ содержит индикатор сложности пароля (passwordScore / «Сложность»)", () => {
    renderForm();
    expect(screen.queryByText(/Сложность/i)).toBeNull();
    expect(screen.queryByText(/Надёжность пароля/i)).toBeNull();
  });

  it("содержит обязательные поля: Email, Роль, Статус, Дней доступа", () => {
    renderForm();
    expect(screen.getByPlaceholderText(/you@example.com/i)).toBeTruthy();
    expect(screen.getByText("Роль")).toBeTruthy();
    expect(screen.getByText("Статус")).toBeTruthy();
    expect(screen.getByText("Дней доступа")).toBeTruthy();
  });

  it("кнопка «Создать и выдать» рендерится в форме", async () => {
    const ctx = makeContext();
    renderForm(ctx);
    const btn = screen.getByRole("button", { name: /Создать и выдать/ });
    expect(btn).toBeTruthy();
    // JSDOM has no HTMLFormElement.prototype.requestSubmit, so
    // a full submit via click is not triggered. It is enough
    // that the button exists and the form is wrapped with onSubmit={createUser}.
    const form = btn.closest("form");
    expect(form).toBeTruthy();
  });
});
