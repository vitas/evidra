import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, beforeEach } from "vitest";
import { ThemeToggle } from "../../src/components/ThemeToggle";

describe("ThemeToggle", () => {
  beforeEach(() => {
    document.documentElement.setAttribute("data-theme", "light");
    localStorage.clear();
  });

  it("describes the action available in the light theme", () => {
    render(<ThemeToggle />);
    const button = screen.getByRole("button", {
      name: "Switch to dark theme",
    });
    expect(button).toHaveAttribute("title", "Switch to dark theme");
  });

  it("describes the next action after switching to the dark theme", async () => {
    render(<ThemeToggle />);
    await userEvent.click(
      screen.getByRole("button", { name: "Switch to dark theme" }),
    );

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    const button = screen.getByRole("button", {
      name: "Switch to light theme",
    });
    expect(button).toHaveAttribute("title", "Switch to light theme");
  });
});
