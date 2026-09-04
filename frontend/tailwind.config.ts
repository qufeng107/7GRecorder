import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        border: "hsl(214 18% 84%)",
        panel: "hsl(0 0% 100%)",
        ink: "hsl(220 22% 12%)",
        muted: "hsl(218 12% 43%)",
        accent: "hsl(173 60% 34%)",
        warn: "hsl(38 92% 50%)",
        danger: "hsl(0 70% 48%)"
      }
    }
  },
  plugins: []
} satisfies Config;
