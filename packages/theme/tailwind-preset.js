// Shared Tailwind v3 preset for CrossTalk SPAs.
// Maps semantic utility names to CSS custom properties from theme.css.
// Colors are full values (oklch/hex/color-mix), not HSL channel triples.
//
// Usage (tailwind.config.{js,ts}):
//   import themePreset from "@crosstalk/theme/preset";
//   export default { presets: [themePreset], content: [...] };

/** @type {Partial<import('tailwindcss').Config>} */
const preset = {
  darkMode: ["class", '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        border: "var(--border)",
        input: "var(--input)",
        ring: "var(--ring)",
        background: "var(--background)",
        foreground: "var(--foreground)",
        card: {
          DEFAULT: "var(--card)",
          foreground: "var(--card-foreground)",
        },
        popover: {
          DEFAULT: "var(--popover)",
          foreground: "var(--popover-foreground)",
        },
        primary: {
          DEFAULT: "var(--primary)",
          foreground: "var(--primary-foreground)",
        },
        secondary: {
          DEFAULT: "var(--secondary)",
          foreground: "var(--secondary-foreground)",
        },
        muted: {
          DEFAULT: "var(--muted)",
          foreground: "var(--muted-foreground)",
        },
        accent: {
          DEFAULT: "var(--accent)",
          foreground: "var(--accent-foreground)",
        },
        destructive: {
          DEFAULT: "var(--destructive)",
          foreground: "var(--destructive-foreground)",
        },
        status: {
          ok: "var(--status-ok)",
          warning: "var(--status-warning)",
          danger: "var(--status-danger)",
          info: "var(--status-info)",
        },
      },
      borderRadius: {
        none: "0",
        sm: "var(--house-radius-sm)",
        DEFAULT: "var(--house-radius-md)",
        md: "var(--house-radius-md)",
        lg: "var(--house-radius-md)",
        full: "var(--house-radius-pill)",
      },
      fontFamily: {
        sans: ["var(--house-font-product)"],
        serif: ["var(--house-font-editorial)"],
        mono: ["var(--house-font-technical)"],
      },
      spacing: {
        "house-1": "var(--house-space-1)",
        "house-2": "var(--house-space-2)",
        "house-3": "var(--house-space-3)",
        "house-4": "var(--house-space-4)",
        "house-6": "var(--house-space-6)",
        "house-8": "var(--house-space-8)",
        "house-12": "var(--house-space-12)",
      },
      minHeight: {
        control: "var(--house-control-height)",
        row: "var(--house-row-height)",
        touch: "var(--house-target-mobile)",
      },
      boxShadow: {
        modal: "var(--house-shadow-modal)",
        toast: "var(--house-shadow-toast)",
        none: "none",
      },
    },
  },
};

export default preset;
