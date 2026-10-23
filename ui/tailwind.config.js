/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        display: ["Inter Tight", "Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        editorial: ["Instrument Serif", "Georgia", "ui-serif", "serif"],
        body: ["Inter Tight", "Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["IBM Plex Mono", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      colors: {
        ink: {
          950: "#070806",
          900: "#0D0F0C",
          800: "#141713",
          700: "#1E221C",
          600: "#2B3028",
          500: "#6D7368",
          400: "#8E9488",
          300: "#B7BCB1",
          200: "#DADDD3",
          100: "#ECEEE7",
          50: "#F7F8F2",
        },
        mint: {
          600: "#A9CC2E",
          500: "#C9EF4A",
          400: "#D9F573",
          300: "#E8FBA8",
        },
        amber: {
          600: "#D99A1D",
          500: "#F4B942",
          400: "#FFD166",
          300: "#FFE4A6",
        },
        ember: {
          600: "#E8472A",
          500: "#FF5C35",
          400: "#FF8769",
          300: "#FFB39F",
        },
        sky: {
          600: "#6D8F8D",
          500: "#8FB4B2",
          400: "#B4CFCB",
          300: "#D4E4E0",
        },
        bone: "#F4F1EA",
      },
      boxShadow: {
        glow: "0 0 0 1px rgba(201,239,74,0.18), 0 18px 50px -24px rgba(201,239,74,0.35)",
        card: "inset 0 1px 0 rgba(255,255,255,0.035), 0 30px 60px -50px rgba(0,0,0,0.95)",
        lift: "inset 0 1px 0 rgba(255,255,255,0.05), 0 24px 48px -28px rgba(0,0,0,0.9)",
      },
      keyframes: {
        "fade-in-up": {
          "0%": { opacity: "0", transform: "translateY(12px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        "fade-in": {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        "slide-left": {
          "0%": { opacity: "0", transform: "translateX(28px)" },
          "100%": { opacity: "1", transform: "translateX(0)" },
        },
        marquee: {
          "0%": { transform: "translateX(0)" },
          "100%": { transform: "translateX(-50%)" },
        },
        "pulse-dot": {
          "0%, 100%": { opacity: "1", transform: "scale(1)" },
          "50%": { opacity: "0.35", transform: "scale(0.8)" },
        },
        shimmer: {
          "0%": { backgroundPosition: "-400px 0" },
          "100%": { backgroundPosition: "400px 0" },
        },
      },
      animation: {
        "fade-in-up": "fade-in-up 650ms cubic-bezier(.16,1,.3,1) both",
        "fade-in": "fade-in 250ms ease-out both",
        "slide-left": "slide-left 420ms cubic-bezier(.16,1,.3,1) both",
        marquee: "marquee 32s linear infinite",
        "pulse-dot": "pulse-dot 2.4s ease-in-out infinite",
        shimmer: "shimmer 1.6s linear infinite",
      },
    },
  },
  plugins: [],
};
