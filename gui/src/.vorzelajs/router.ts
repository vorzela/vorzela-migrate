// Client router built from the generated route tree.
import { createRouter } from '@vorzelajs/router'

import { routeTree } from '@/runtime/routeTree.gen'

export const router = createRouter({
  routes: routeTree,
  seo: {
    title: 'VorzelaJs Client File Router',
    description: 'A client-rendered app with file-based routes.',
  },
})
