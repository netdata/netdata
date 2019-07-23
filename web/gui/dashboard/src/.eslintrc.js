module.exports = {
  "extends": [
    "react-app",
    "airbnb",
  ],
  "rules" : {
    "indent": ["error", 2],
    "semi": ["error", "never"],
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
  }
}
