// Explicit regeneration for typecheck and build scripts.
import { generateRouteTree } from '@vorzelajs/router/vite'

generateRouteTree('src/routes', 'src/.vorzelajs/routeTree.gen.ts')
