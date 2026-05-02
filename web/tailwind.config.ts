import type { Config } from 'tailwindcss'

const config: Config = {
  darkMode: ['class'],
  content: [
    './index.html',
    './src/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'Arial', 'sans-serif'],
      },
      colors: {
        background: '#ffffff',
        surface: '#f5f5f5',
        'surface-soft': '#f7f8fa',
        'text-main': '#111111',
        'text-muted': '#666666',
        border: '#e6e6e6',
        primary: {
          DEFAULT: '#ffcc00',
          hover: '#f2c200',
          foreground: '#111111',
        },
      },
      borderRadius: {
        '4xl': '2rem',
      },
    },
  },
  plugins: [],
}

export default config
