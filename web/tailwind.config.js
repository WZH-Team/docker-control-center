/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./static/**/*.{html,js}"],
  theme: {
    extend: {
      colors: {
        // 蓝绿（teal）主题色阶。
        brand: {
          50: "#effcf9",
          100: "#c8f7ee",
          200: "#93eddf",
          300: "#5cdcc9",
          400: "#2cc2b0",
          500: "#12a798",
          600: "#0b8578",
          700: "#0c6a60",
          800: "#0d544d",
          900: "#0c453f",
          950: "#022b28"
        }
      },
      fontFamily: {
        sans: [
          "ui-sans-serif",
          "system-ui",
          "-apple-system",
          "Segoe UI",
          "Roboto",
          "Helvetica Neue",
          "Arial",
          "Noto Sans SC",
          "PingFang SC",
          "Microsoft YaHei",
          "sans-serif"
        ],
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Consolas",
          "Liberation Mono",
          "monospace"
        ]
      },
      boxShadow: {
        card: "0 1px 2px 0 rgb(15 23 42 / 0.05), 0 1px 3px 0 rgb(15 23 42 / 0.06)",
        pop: "0 10px 38px -10px rgb(2 43 40 / 0.22), 0 4px 12px -4px rgb(2 43 40 / 0.12)"
      }
    }
  },
  plugins: []
};
