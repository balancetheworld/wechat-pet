import antfu from '@antfu/eslint-config'

export default antfu(
  {
    react: true,
    typescript: true,
    formatters: false,
  },
  {
    ignores: ['**/node_modules/**', '**/dist/**', '**/.taro/**', '**/.temp/**', '**/coverage/**'],
  },
  {
    name: 'client/globals-and-rules',
    languageOptions: {
      globals: {
        App: 'readonly',
        Behavior: 'readonly',
        Component: 'readonly',
        Page: 'readonly',
        getApp: 'readonly',
        getCurrentPages: 'readonly',
        wx: 'readonly',
      },
    },
    rules: {
      'react/react-in-jsx-scope': 'off',
      'react/jsx-uses-react': 'off',
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
    },
  },
)
