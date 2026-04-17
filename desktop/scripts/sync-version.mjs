import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const projectDir = path.resolve(__dirname, '..')
const cargoTomlPath = path.join(projectDir, 'src-tauri', 'Cargo.toml')
const packageJsonPath = path.join(projectDir, 'package.json')

function readCargoVersion(content) {
  const packageSectionMatch = content.match(/^\[package\][\s\S]*?(?=^\[[^\]]+\]|\Z)/m)
  if (!packageSectionMatch)
    throw new Error('Cargo.toml missing [package] section')

  const versionMatch = packageSectionMatch[0].match(/^version\s*=\s*"([^"]+)"/m)
  if (!versionMatch)
    throw new Error('Cargo.toml missing package version')

  return versionMatch[1]
}

const cargoToml = await readFile(cargoTomlPath, 'utf8')
const cargoVersion = readCargoVersion(cargoToml)
const packageJson = JSON.parse(await readFile(packageJsonPath, 'utf8'))

if (packageJson.version !== cargoVersion) {
  packageJson.version = cargoVersion
  await writeFile(packageJsonPath, `${JSON.stringify(packageJson, null, 2)}\n`, 'utf8')
  console.log(`synced package.json version to ${cargoVersion}`)
} else {
  console.log(`package.json version already matches ${cargoVersion}`)
}