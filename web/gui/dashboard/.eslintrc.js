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
    "quotes": ["error", "double"],
    "react/jsx-filename-extension": [
      1,
      {
        "extensions": [
          ".tsx"
        ]
      }
    ],
    "no-underscore-dangle": ["error", { "allow": ["__REDUX_DEVTOOLS_EXTENSION__"] }],
    "import/prefer-default-export": 0,
    "@typescript-eslint/no-unused-vars": "error"
  },
  "settings": {
    "import/resolver": {
      "node": {
        "extensions": [".js", ".jsx", ".ts", ".tsx", ".d.ts"],
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
