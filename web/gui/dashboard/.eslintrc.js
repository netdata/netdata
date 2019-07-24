module.exports = {
  "parser": "@typescript-eslint/parser",
  "plugins": [
    "@typescript-eslint",
  ],
  "env": {
    "browser": true,
  },
  "extends": [
    "airbnb",
  ],
  "rules" : {
    "indent": ["error", 2],
    "semi": ["error", "never"],
    "@typescript-eslint/semi": ["error", "never"],
    "react/jsx-filename-extension": [
      1,
      {
        "extensions": [
          ".tsx"
        ]
      }
    ],
  },
  "settings": {
    "import/resolver": {
      "node": {
        "extensions": [".js", ".jsx", ".ts", ".tsx"],
        "paths": ["src"]
      }
    }
  },
  "overrides": [
    {
      "files": ["*.test.*"],
      "env": {
        "jest": true,
      }
    }
  ]
}
