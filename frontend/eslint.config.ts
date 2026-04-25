import antfu, { isInEditorEnv } from '@antfu/eslint-config'

export default antfu({
  isInEditor: isInEditorEnv(),
  vue: true,
  typescript: true,
  ignores: ['dist', 'node_modules'],
})
