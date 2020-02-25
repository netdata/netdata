module.exports = function atob(str) {
  return Buffer.from(str, 'base64').toString('binary')
}
