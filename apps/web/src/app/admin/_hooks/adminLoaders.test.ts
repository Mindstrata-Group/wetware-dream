import { describe, expect, it, vi, afterEach } from "vitest";
import { createAdminLoaders } from "./adminLoaders";
import { apiFetch } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  apiFetch: vi.fn(),
}));

function makeContext(overrides: Record<string, unknown> = {}) {
  return {
    userMeta: { total: 0, limit: 50, offset: 0 },
    modeMeta: { total: 0, limit: 50, offset: 0 },
    userQ: "",
    modeQ: "",
    tariffSort: "created_desc",
    promoQ: "",
    exportOptionsLoaded: false,
    setProfile: vi.fn(),
    setStatus: vi.fn(),
    setAdminStats: vi.fn(),
    setUsers: vi.fn(),
    setUserMeta: vi.fn(),
    setModes: vi.fn(),
    setModeMeta: vi.fn(),
    setTariffs: vi.fn(),
    setGroups: vi.fn(),
    setPromocodes: vi.fn(),
    setExportUsers: vi.fn(),
    setExportModes: vi.fn(),
    setExportPromocodes: vi.fn(),
    setExportOptionsLoaded: vi.fn(),
    setSummaryPrompts: vi.fn(),
    setExportFilters: vi.fn(),
    setModelStats: vi.fn(),
    setOrchestrationPrompt: vi.fn(),
    setDialogSummaryPrompt: vi.fn(),
    setAISettings: vi.fn(),
    setGuardrail: vi.fn(),
    handleError: vi.fn(),
    ...overrides,
  };
}

describe("adminLoaders", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("загружает сохраненный guardrail и сохраняет остальные поля формы", async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ ok: true, text: "Живой стиль ответа" });
    const ctx = makeContext();
    const loaders = createAdminLoaders(ctx as never);

    await loaders.loadGuardrail();

    expect(apiFetch).toHaveBeenCalledWith("/api/admin/modes/guardrail");
    expect(ctx.setGuardrail).toHaveBeenCalledTimes(1);
    const updater = vi.mocked(ctx.setGuardrail).mock.calls[0][0] as (value: {
      text: string;
      testModel: string;
      replaceAll: boolean;
      find: string;
      replacement: string;
    }) => {
      text: string;
      testModel: string;
      replaceAll: boolean;
      find: string;
      replacement: string;
    };
    expect(updater({ text: "старый", testModel: "m", replaceAll: true, find: "a", replacement: "b" })).toEqual({
      text: "Живой стиль ответа",
      testModel: "m",
      replaceAll: true,
      find: "a",
      replacement: "b",
    });
  });
});
