import type { Dictionary } from "@/lib/i18n/types"

type BlogText = {
  hero: { title: string; subtitle: string }
  closed: { title: string; text: string }
  emptyState: string
  allTag: string
  readLabel: string
  readArrowLabel: string
  paginationLabel: string
  subscribe: { title: string; text: string; buttonLabel: string }
  defaultReadMinutes: string
  article: {
    backLabel: string
    notFoundTitle: string
    notFoundText: string
    ctaText: string
    chatLabel: string
    loginLabel: string
    readingSuffix: string
  }
}

export const blogText: Dictionary<BlogText> = {
  ru: {
    hero: {
      title: "Блог Mindstrata",
      subtitle: "Короткие разборы методик, выступлений и сценариев применения ИИ.",
    },
    closed: {
      title: "Блог пока закрыт",
      text: "Раздел скоро откроется.",
    },
    emptyState: "В этой теме пока нет опубликованных статей.",
    allTag: "Все",
    readLabel: "Читать",
    readArrowLabel: "Читать →",
    paginationLabel: "Пагинация блога",
    subscribe: {
      title: "Раз в две недели — один полезный разбор",
      text: "Без новостной ленты. Только методика, сценарий или практический вывод.",
      buttonLabel: "Войти и попробовать",
    },
    defaultReadMinutes: "5 мин",
    article: {
      backLabel: "Все статьи",
      notFoundTitle: "Статья не найдена",
      notFoundText: "Она могла быть снята с публикации или ещё не опубликована.",
      ctaText: "Можно сразу попробовать режимы на своём вопросе.",
      chatLabel: "Открыть чат",
      loginLabel: "Войти",
      readingSuffix: "чтения",
    },
  },
  en: {
    hero: {
      title: "Mindstrata Blog",
      subtitle: "Short breakdowns of methods, talks, and AI use-case scenarios.",
    },
    closed: {
      title: "The blog is currently closed",
      text: "This section will open soon.",
    },
    emptyState: "There are no published articles in this topic yet.",
    allTag: "All",
    readLabel: "Read",
    readArrowLabel: "Read →",
    paginationLabel: "Blog pagination",
    subscribe: {
      title: "Every two weeks — one useful breakdown",
      text: "No news feed. Just a method, scenario, or practical takeaway.",
      buttonLabel: "Log in and try it",
    },
    defaultReadMinutes: "5 min",
    article: {
      backLabel: "All articles",
      notFoundTitle: "Article not found",
      notFoundText: "It may have been unpublished or not published yet.",
      ctaText: "You can try the modes right away with your own question.",
      chatLabel: "Open chat",
      loginLabel: "Log in",
      readingSuffix: "read",
    },
  },
}
