import { fireEvent, render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CodeBlock } from "../../src/components/CodeBlock";

const writeText = vi.fn();

describe("CodeBlock", () => {
  beforeEach(() => {
    writeText.mockReset();
    writeText.mockResolvedValue(undefined);
    // jsdom has no clipboard; user-event would install its own polyfill on
    // setup(), so define the mock directly and click with fireEvent.
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
  });

  it("renders code content", () => {
    render(<CodeBlock code="echo hello" />);
    expect(screen.getByText("echo hello")).toBeInTheDocument();
  });

  it("copies code to clipboard on button click", async () => {
    render(<CodeBlock code="echo hello" />);
    fireEvent.click(screen.getByRole("button", { name: /copy/i }));
    expect(writeText).toHaveBeenCalledWith("echo hello");
    expect(await screen.findByRole("status")).toHaveTextContent(/copied/i);
  });

  it("reports clipboard failure without an unhandled rejection", async () => {
    writeText.mockRejectedValueOnce(new Error("denied"));

    render(<CodeBlock code="echo hello" />);
    fireEvent.click(screen.getByRole("button", { name: /copy/i }));

    expect(await screen.findByRole("status")).toHaveTextContent(/copy failed/i);
  });
});
