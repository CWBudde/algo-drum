import { beforeEach, describe, expect, it, vi } from "vitest";

const rootMock = vi.hoisted(() => ({ render: vi.fn() }));

vi.mock("react-dom/client", () => ({
  createRoot: vi.fn(() => rootMock),
}));
vi.mock("./App", () => ({ default: () => null }));

describe("browser entry point", () => {
  beforeEach(() => {
    vi.resetModules();
    rootMock.render.mockClear();
    document.body.innerHTML = '<div id="root"></div>';
  });

  it("mounts the React application into the root element", async () => {
    await import("./main");
    expect(rootMock.render).toHaveBeenCalledOnce();
  });
});
