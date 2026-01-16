import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

import en from '@/locales/en.json';
import zh from '@/locales/zh.json';

// 获取浏览器语言
function getBrowserLanguage(): string {
  const browserLang = navigator.language || navigator.languages?.[0] || 'en';
  // 简化语言代码，如 zh-CN -> zh, en-US -> en
  const lang = browserLang.split('-')[0];
  return ['zh', 'en'].includes(lang) ? lang : 'en';
}

// 获取存储的语言或浏览器语言
function getInitialLanguage(): string {
  const stored = localStorage.getItem('maxx-ui-language');
  if (stored && ['zh', 'en'].includes(stored)) {
    return stored;
  }
  return getBrowserLanguage();
}

i18n
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      zh: { translation: zh },
    },
    lng: getInitialLanguage(),
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false, // React 已经处理了 XSS
    },
  });

// 语言变化时保存到 localStorage
i18n.on('languageChanged', (lng) => {
  localStorage.setItem('maxx-ui-language', lng);
});

export default i18n;
