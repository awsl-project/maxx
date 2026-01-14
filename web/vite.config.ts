import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";
import tailwindcss from "@tailwindcss/vite";
import { execSync } from "child_process";

// Get git commit hash
function getGitCommit(): string {
	try {
		return execSync("git rev-parse --short HEAD").toString().trim();
	} catch {
		return "unknown";
	}
}

// https://vite.dev/config/
export default defineConfig({
	plugins: [react(), tailwindcss()],
	define: {
		__APP_VERSION__: JSON.stringify(process.env.npm_package_version || "dev"),
		__APP_COMMIT__: JSON.stringify(process.env.VITE_COMMIT || getGitCommit()),
		__APP_BUILD_TIME__: JSON.stringify(new Date().toISOString()),
	},
	resolve: {
		alias: {
			"@": path.resolve(__dirname, "./src"),
		},
	},
	build: {
		rollupOptions: {
			external: [
				"@wailsio/runtime",
				// Wails 生成的绑定只在桌面模式下存在，Docker 构建时需要排除
				/^@\/wailsjs\/.*/,
			],
		},
	},
	server: {
		port: 3000,
		proxy: {
			"/admin": {
				target: "http://localhost:9880",
				changeOrigin: true,
			},
			"/antigravity": {
				target: "http://localhost:9880",
				changeOrigin: true,
			},
			"/ws": {
				target: "http://localhost:9880",
				ws: true,
			},
			"/health": {
				target: "http://localhost:9880",
				changeOrigin: true,
			},
		},
	},
});
