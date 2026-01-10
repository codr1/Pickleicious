// web/tailwind.config.js
/** @type {import('tailwindcss').Config} */
module.exports = {
    darkMode: "class",
    content: [
        "../internal/templates/components/**/*.templ",
        "../internal/templates/components/**/*_templ.go",
        "../internal/templates/layouts/**/*.templ",
        "../internal/templates/layouts/**/*_templ.go"
    ],
    safelist: [
        "dark:hidden",
        "dark:block",
    ],
    theme: {
        extend: {
            colors: {
                border: "hsl(var(--border))",
                
                input: "hsl(var(--input))",
                ring: "hsl(var(--ring))",
                background: "hsl(var(--background))",
                foreground: "hsl(var(--foreground))",
                primary: {
                    DEFAULT: "var(--theme-primary)",
                    foreground: "hsl(var(--primary-foreground))",
                },
                secondary: {
                    DEFAULT: "var(--theme-secondary)",
                    foreground: "hsl(var(--secondary-foreground))",
                },
                destructive: {
                    DEFAULT: "hsl(var(--destructive))",
                    foreground: "hsl(var(--destructive-foreground))",
                },
                muted: {
                    DEFAULT: "hsl(var(--muted))",
                    foreground: "hsl(var(--muted-foreground))",
                },
                accent: {
                    DEFAULT: "var(--theme-accent)",
                    foreground: "hsl(var(--accent-foreground))",
                },
                tertiary: "var(--theme-tertiary)",
                highlight: "var(--theme-highlight)",
                popover: {
                    DEFAULT: "hsl(var(--popover))",
                    foreground: "hsl(var(--popover-foreground))",
                },
                card: {
                    DEFAULT: "hsl(var(--card))",
                    foreground: "hsl(var(--card-foreground))",
                },
            },
            borderRadius: {
                lg: "var(--radius)",
                md: "calc(var(--radius) - 2px)",
                sm: "calc(var(--radius) - 4px)",
            },
        },
    },
    plugins: [],
};
