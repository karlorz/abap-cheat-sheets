import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const apiTarget = process.env.PTD_API_ORIGIN || "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true
      }
    }
  }
});
