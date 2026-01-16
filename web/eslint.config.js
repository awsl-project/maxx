import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist', 'src/wailsjs']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    rules: {
      // 允许在 effect 中调用 setState（定时器更新等场景需要）
      'react-hooks/set-state-in-effect': 'off',
      // 允许在渲染期间访问 ref（某些同步更新场景需要）
      'react-hooks/refs': 'off',
      // 允许文件导出非组件（常量、工具函数等）
      'react-refresh/only-export-components': 'off',
      // 允许使用 @ts-ignore
      '@typescript-eslint/ban-ts-comment': 'off',
      // 允许空接口
      '@typescript-eslint/no-empty-object-type': 'off',
      // 允许使用 any 类型
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
])
