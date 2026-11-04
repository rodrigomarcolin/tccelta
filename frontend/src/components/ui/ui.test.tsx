import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Button } from "./Button";
import { Badge } from "./Badge";
import { Indicator } from "./Indicator";
import { ThemeToggle, saveTheme, ThemeProvider, useTheme } from "./Theme";

describe("Button", () => {
  it("variants, default type button and icon", () => {
    render(
      <Button variant="secondary" fullWidth icon={<i data-testid="i" />}>
        Ok
      </Button>,
    );
    const b = screen.getByRole("button", { name: "Ok" });
    expect(b).toHaveAttribute("type", "button");
    expect(b).toHaveClass("button-secondary", "button-block");
    expect(screen.getByTestId("i")).toBeInTheDocument();
  });
});

describe("Badge", () => {
  it("pulses only when requested", () => {
    const { container, rerender } = render(<Badge tone="live">Ao vivo</Badge>);
    expect(container.querySelector(".dot-pulse")).toBeNull();
    rerender(
      <Badge tone="alert" pulse>
        Ao vivo
      </Badge>,
    );
    expect(container.querySelector(".dot-pulse")).not.toBeNull();
    expect(container.firstChild).toHaveClass("badge-alert");
  });
});

describe("Indicator", () => {
  it("shows label, mono value, unit and note", () => {
    render(<Indicator label="Distância" value="3,5" unit="km" note="estimado" />);
    expect(screen.getByText("Distância")).toBeInTheDocument();
    expect(screen.getByText("3,5")).toHaveClass("data-value");
    expect(screen.getByText("km")).toHaveClass("unit");
    expect(screen.getByText("estimado")).toBeInTheDocument();
  });
  it("omits unit and note when absent", () => {
    const { container } = render(<Indicator label="X" value="1" />);
    expect(container.querySelector(".unit")).toBeNull();
    expect(container.querySelector(".indicator-note")).toBeNull();
  });
});

describe("theme", () => {
  beforeEach(() => {
    document.cookie = "theme=; max-age=0; path=/";
    document.documentElement.removeAttribute("data-theme");
  });

  it("toggle uses aria-pressed, persists in a cookie and swaps the attribute without reloading", async () => {
    function Probe() {
      return <span data-testid="theme">{useTheme()}</span>;
    }
    render(
      <ThemeProvider initial="dark">
        <ThemeToggle />
        <Probe />
      </ThemeProvider>,
    );
    const button = screen.getByRole("button", { name: "Tema claro" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(button);
    expect(button).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("theme")).toHaveTextContent("light");
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.cookie).toContain("theme=light");
    await userEvent.click(button);
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(document.cookie).toContain("theme=dark");
  });

  it("saveTheme accepts another document", () => {
    const doc = document.implementation.createHTMLDocument("x");
    saveTheme("light", doc);
    expect(doc.documentElement.dataset.theme).toBe("light");
  });

  it("without a provider the default theme is dark", () => {
    function Probe() {
      return <span data-testid="t">{useTheme()}</span>;
    }
    render(<Probe />);
    expect(screen.getByTestId("t")).toHaveTextContent("dark");
    fireEvent.click(document.body);
    vi.restoreAllMocks();
  });
});
