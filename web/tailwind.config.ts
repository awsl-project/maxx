import type { Config } from "tailwindcss";

export default {
	content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
	darkMode: "class",
	theme: {
		extend: {
			width: {
				"screen-xs": "320px",
				"screen-sm": "640px",
				"screen-md": "768px",
				"screen-lg": "1024px",
				"screen-xl": "1280px",
				"screen-2xl": "1536px",
			},
			spacing: {
				xs: "var(--spacing-xs)",
				sm: "var(--spacing-sm)",
				md: "var(--spacing-md)",
				lg: "var(--spacing-lg)",
				xl: "var(--spacing-xl)",
				xxl: "var(--spacing-xxl)",
			},
			borderRadius: {
				sm: "4px",
				md: "8px",
				lg: "12px",
			},
			fontSize: {
				caption: ["11px", { lineHeight: "1.4" }],
				body: ["13px", { lineHeight: "1.5" }],
				headline: ["15px", { lineHeight: "1.4" }],
				title3: ["17px", { lineHeight: "1.3" }],
				title2: ["20px", { lineHeight: "1.2" }],
				title1: ["24px", { lineHeight: "1.2" }],
				"large-title": ["28px", { lineHeight: "1.1" }],
			},
			colors: {
				background: "var(--color-background)",
				"surface-primary": "var(--color-card)",
				"surface-secondary": "var(--color-secondary)",
				"surface-hover": "var(--color-accent)",
				border: "var(--color-border)",
				"text-primary": "var(--color-foreground)",
				"text-secondary": "var(--color-muted-foreground)",
				"text-muted": "var(--color-muted-foreground)",
				accent: {
					DEFAULT: "var(--color-primary)",
					hover: "var(--color-primary)",
				},
				success: "#4EC9B0",
				warning: "#DDB359",
				error: "#F14C4C",
				info: "#4FC1FF",

				// Provider 品牌色
				provider: {
					anthropic: "var(--color-provider-anthropic)",
					openai: "var(--color-provider-openai)",
					deepseek: "var(--color-provider-deepseek)",
					google: "var(--color-provider-google)",
					azure: "var(--color-provider-azure)",
					aws: "var(--color-provider-aws)",
					cohere: "var(--color-provider-cohere)",
					mistral: "var(--color-provider-mistral)",
					custom: "var(--color-provider-custom)",
					antigravity: "var(--color-provider-antigravity)",
				},

				// Client 品牌色
				client: {
					claude: "var(--color-client-claude)",
					openai: "var(--color-client-openai)",
					codex: "var(--color-client-codex)",
					gemini: "var(--color-client-gemini)",
				},
			},
			boxShadow: {
				card: "0 2px 8px rgba(0, 0, 0, 0.08)",
				"card-hover": "0 4px 12px rgba(0, 0, 0, 0.12)",
			},
			animation: {
				snowfall: "snowfall 8s linear infinite",
				"spin-slow": "spin 3s linear infinite",
			},
			keyframes: {
				snowfall: {
					"0%": {
						transform: "translateY(-10px) translateX(-10px) rotate(0deg)",
						opacity: "0",
					},
					"20%": { opacity: "1" },
					"100%": {
						transform: "translateY(8rem) translateX(10px) rotate(180deg)",
						opacity: "0",
					},
				},
			},
		},
	},
} satisfies Config;
