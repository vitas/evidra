import { useTheme } from "../hooks/useTheme";

export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const actionLabel =
    theme === "dark" ? "Switch to light theme" : "Switch to dark theme";

  return (
    <button
      onClick={toggle}
      aria-label={actionLabel}
      title={actionLabel}
      className="bg-transparent border border-border rounded-md w-8 h-8 cursor-pointer text-fg-muted flex items-center justify-center transition-all hover:border-accent hover:text-fg"
    >
      {theme === "dark" ? "\u2600" : "\u263D"}
    </button>
  );
}
