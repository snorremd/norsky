import type { Config } from 'tailwindcss';

const config: Config = {
  content: [
    './index.html',
    './frontend/**/*.{js,ts,jsx,tsx,css,md,mdx,html,json,scss}',
  ],
  darkMode: 'class',
  theme: {
    extend: {},
  },
  plugins: [],
};

export default config;
