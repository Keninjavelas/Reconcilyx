/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        display: ["Fraunces", "ui-serif", "Georgia", "serif"],
        body: ["Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      colors: {
        ink: {
          950: "#0B0F14",
          900: "#111827",
          800: "#1A2130",
          700: "#252E40",
          600: "#334155",
          500: "#475569",
          100: "#C9D1DC",
          50: "#F4F6FA",
        },
        mint: {
          500: "#34D399",
          400: "#4ADE80",
          300: "#86EFAC",
          600: "#10B981",
        },
        amber: {
          500: "#F59E0B",
          400: "#FBBF24",
          600: "#D97706",
        },
        ember: {
          500: "#EF4444",
          400: "#F87171",
          600: "#DC2626",
        },
        sky: {
          500: "#38BDF8",
          400: "#7DD3FC",
          600: "#0EA5E9",
        },
      },
      boxShadow: {
        glow: "0 0 0 1px rgba(52,211,153,0.15), 0 12px 40px -12px rgba(16,185,129,0.35)",
        card: "0 1px 0 rgba(255,255,255,0.03) inset, 0 10px 30px -20px rgba(0,0,0,0.6)",
      },
      backgroundImage: {
        grain:
          "radial-gradient(rgba(255,255,255,0.04) 1px, transparent 1px)",
      },
      backgroundSize: {
        grain: "3px 3px",
      },
      keyframes: {
        "fade-in-up": {
          "0%": { opacity: "0", transform: "translateY(8px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
      },
      animation: {
        "fade-in-up": "fade-in-up 450ms cubic-bezier(.2,.7,.2,1) both",
      },
    },
  },
  plugins: [],
};
