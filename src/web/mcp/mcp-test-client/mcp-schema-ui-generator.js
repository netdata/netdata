/**
 * MCP Schema UI Generator
 * A lightweight JSON Schema form generator specifically designed for MCP tool parameters
 */

class MCPSchemaUIGenerator {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            onChange: options.onChange || (() => {}),
            showDescriptions: options.showDescriptions !== false,
            showTooltips: options.showTooltips !== false,
            ...options
        };
        this.value = {};
        this.schema = null;
        this.errors = {};
        this.fieldElements = new Map();
        this._changeTimer = null;
    }

    /**
     * Set the schema and render the form
     */
    setSchema(schema, initialValue = {}) {
        // Validate schema against MCP spec before using it
        this._validateMCPSchema(schema);
        
        this.schema = schema;
        this.value = this._deepClone(initialValue);
        this.errors = {};
        this.render();
    }

    /**
     * Get the current form value
     */
    getValue() {
        const cleanedValue = {};
        for (const [key, value] of Object.entries(this.value)) {
            // Filter out anyOf helper keys (they have _anyof_ in their name)
            if (!key.includes('_anyof_')) {
                cleanedValue[key] = value;
            }
        }
        return this._deepClone(cleanedValue);
    }

    /**
     * Set the form value
     */
    setValue(value) {
        this.value = this._deepClone(value);
        this._updateFieldValues();
    }

    /**
     * Validate the current value against the schema
     */
    validate() {
        this.errors = {};
        if (!this.schema) return true;

        // Check required fields
        if (this.schema.required && Array.isArray(this.schema.required)) {
            for (const field of this.schema.required) {
                if (!this.value.hasOwnProperty(field) || 
                    this.value[field] === null || 
                    this.value[field] === undefined ||
                    (typeof this.value[field] === 'string' && this.value[field].trim() === '')) {
                    this.errors[field] = 'This field is required';
                }
            }
        }

        // Validate individual fields
        if (this.schema.properties) {
            for (const [key, propSchema] of Object.entries(this.schema.properties)) {
                const value = this.value[key];
                const error = this._validateField(value, propSchema);
                if (error) {
                    this.errors[key] = error;
                }
            }
        }

        this._updateErrorDisplay();
        return Object.keys(this.errors).length === 0;
    }

    /**
     * Render the form
     */
    render() {
        this.container.innerHTML = '';
        this.fieldElements.clear();

        if (!this.schema || !this.schema.properties) {
            this.container.innerHTML = '<p>No schema provided</p>';
            return;
        }

        // Add title if present
        if (this.schema.title) {
            const title = document.createElement('h4');
            title.className = 'mcp-form-title';
            title.textContent = this.schema.title;
            this.container.appendChild(title);
        }

        // Add description if present
        if (this.schema.description && this.options.showDescriptions) {
            const desc = document.createElement('p');
            desc.className = 'mcp-form-description';
            desc.textContent = this.schema.description;
            this.container.appendChild(desc);
        }

        // Create form wrapper
        const form = document.createElement('div');
        form.className = 'mcp-form';
        this.container.appendChild(form);

        // Render each property
        for (const [key, propSchema] of Object.entries(this.schema.properties)) {
            // Initialize with default value if not already set
            if (this.value[key] === undefined && propSchema.default !== undefined) {
                this.value[key] = propSchema.default;
            }
            
            const fieldContainer = this._renderField(key, propSchema, this.value[key]);
            form.appendChild(fieldContainer);
            this.fieldElements.set(key, fieldContainer);
        }

        // Add CSS if not already present
        this._injectStyles();
    }

    /**
     * Render a single field
     */
    _renderField(key, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-field';
        container.dataset.field = key;

        // Label
        const labelWrapper = document.createElement('div');
        labelWrapper.className = 'mcp-field-label-wrapper';

        const label = document.createElement('label');
        label.className = 'mcp-field-label';
        
        // Add tooltip functionality if description exists
        if (schema.description && this.options.showTooltips) {
            label.className += ' mcp-field-label-hoverable';
            
            // Add hover handlers for proper positioning
            let tooltipEl = null;
            label.addEventListener('mouseenter', (e) => {
                const rect = label.getBoundingClientRect();
                tooltipEl = document.createElement('div');
                tooltipEl.className = 'mcp-tooltip-popup';
                // Convert newlines to <br/> tags
                tooltipEl.innerHTML = schema.description.replace(/\n/g, '<br/>');
                tooltipEl.style.position = 'fixed';
                tooltipEl.style.left = `${rect.left}px`;
                tooltipEl.style.top = `${rect.top - 5}px`;
                tooltipEl.style.transform = 'translateY(-100%)';
                document.body.appendChild(tooltipEl);
            });
            
            label.addEventListener('mouseleave', () => {
                if (tooltipEl) {
                    tooltipEl.remove();
                    tooltipEl = null;
                }
            });
        }
        
        label.textContent = schema.title || key;
        
        // Mark required fields
        if (this.schema.required && this.schema.required.includes(key)) {
            const required = document.createElement('span');
            required.className = 'mcp-field-required';
            required.textContent = '*';
            label.appendChild(required);
        }

        labelWrapper.appendChild(label);

        // Add JSON field name
        const fieldName = document.createElement('span');
        fieldName.className = 'mcp-field-json-name';
        fieldName.textContent = key;
        labelWrapper.appendChild(fieldName);

        container.appendChild(labelWrapper);

        // Description (below label)
        if (schema.description && this.options.showDescriptions && !this.options.showTooltips) {
            const desc = document.createElement('div');
            desc.className = 'mcp-field-description';
            desc.innerHTML = schema.description.replace(/\n/g, '<br/>');
            container.appendChild(desc);
        }

        // Input
        const inputContainer = document.createElement('div');
        inputContainer.className = 'mcp-field-input-container';
        
        const input = this._createInput(key, schema, value);
        inputContainer.appendChild(input);
        container.appendChild(inputContainer);

        // Error message placeholder
        const error = document.createElement('div');
        error.className = 'mcp-field-error';
        container.appendChild(error);

        return container;
    }

    /**
     * Create appropriate input element based on schema
     */
    _createInput(key, schema, value) {
        // Handle enum
        if (schema.enum) {
            return this._createSelect(key, schema, value);
        }

        // Handle type-specific inputs
        switch (schema.type) {
            case 'string':
                return this._createStringInput(key, schema, value);
            case 'number':
            case 'integer':
                return this._createNumberInput(key, schema, value);
            case 'boolean':
                return this._createBooleanInput(key, schema, value);
            case 'array':
                return this._createArrayInput(key, schema, value);
            case 'object':
                return this._createObjectInput(key, schema, value);
            default:
                // Handle anyOf/oneOf
                if (schema.anyOf || schema.oneOf) {
                    return this._createMultiTypeInput(key, schema, value);
                }
                return this._createStringInput(key, schema, value);
        }
    }

    /**
     * Create string input
     */
    _createStringInput(key, schema, value) {
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'mcp-input';
        input.value = value || schema.default || '';
        input.placeholder = schema.placeholder || '';
        
        if (schema.minLength) input.minLength = schema.minLength;
        if (schema.maxLength) input.maxLength = schema.maxLength;
        if (schema.pattern) input.pattern = schema.pattern;

        input.addEventListener('input', () => {
            this.value[key] = input.value;
            this._onChange();
        });

        return input;
    }

    /**
     * Create number input
     */
    _createNumberInput(key, schema, value) {
        const input = document.createElement('input');
        input.type = 'number';
        input.className = 'mcp-input';
        input.value = value !== undefined ? value : (schema.default !== undefined ? schema.default : '');
        
        if (schema.minimum !== undefined) input.min = schema.minimum;
        if (schema.maximum !== undefined) input.max = schema.maximum;
        if (schema.multipleOf !== undefined) input.step = schema.multipleOf;

        input.addEventListener('input', () => {
            const val = input.value === '' ? undefined : Number(input.value);
            this.value[key] = val;
            this._onChange();
        });

        return input;
    }

    /**
     * Create boolean input
     */
    _createBooleanInput(key, schema, value) {
        const wrapper = document.createElement('label');
        wrapper.className = 'mcp-checkbox-wrapper';

        const input = document.createElement('input');
        input.type = 'checkbox';
        input.className = 'mcp-checkbox';
        input.checked = value !== undefined ? value : (schema.default || false);

        input.addEventListener('change', () => {
            this.value[key] = input.checked;
            this._onChange();
        });

        const text = document.createElement('span');
        text.textContent = schema.title || key;

        wrapper.appendChild(input);
        wrapper.appendChild(text);

        return wrapper;
    }

    /**
     * Create select dropdown
     */
    _createSelect(key, schema, value) {
        const select = document.createElement('select');
        select.className = 'mcp-select';

        // Always add empty option if there's no default value
        // This ensures nothing is pre-selected when there's no default
        const hasDefault = schema.default !== undefined;
        const isRequired = this.schema.required && this.schema.required.includes(key);
        
        if (!hasDefault || !isRequired) {
            const emptyOption = document.createElement('option');
            emptyOption.value = '';
            emptyOption.textContent = '-- Not selected --';
            select.appendChild(emptyOption);
        }

        // Add enum options
        for (const option of schema.enum) {
            const optionEl = document.createElement('option');
            optionEl.value = option;
            optionEl.textContent = option;
            select.appendChild(optionEl);
        }
        
        // Set the selected value
        if (value !== undefined && value !== null && value !== '') {
            select.value = value;
        } else if (hasDefault) {
            select.value = schema.default;
            // Also set it in the data model
            this.value[key] = schema.default;
        } else {
            // No value and no default - select the empty option
            select.value = '';
        }

        select.addEventListener('change', () => {
            this.value[key] = select.value || undefined;
            this._onChange();
        });

        return select;
    }

    /**
     * Create array input
     */
    _createArrayInput(key, schema, value) {
        // Check if this is an array of enums - use checkbox interface
        if (schema.items && schema.items.enum && Array.isArray(schema.items.enum)) {
            return this._createArrayEnumCheckboxes(key, schema, value);
        }
        
        // Check if this is a tuple array with oneOf items
        if (schema.items && schema.items.type === 'array' && schema.items.items && schema.items.items.oneOf) {
            return this._createTupleArrayInput(key, schema, value);
        }
        
        // Regular array interface
        const container = document.createElement('div');
        container.className = 'mcp-array-container';

        const items = Array.isArray(value) ? value : (schema.default || []);
        this.value[key] = [...items];

        const itemsContainer = document.createElement('div');
        itemsContainer.className = 'mcp-array-items';

        const renderItems = () => {
            itemsContainer.innerHTML = '';
            this.value[key].forEach((item, index) => {
                const itemEl = this._createArrayItem(key, schema, item, index);
                itemsContainer.appendChild(itemEl);
            });
        };

        renderItems();
        container.appendChild(itemsContainer);

        // Add button
        const addButton = document.createElement('button');
        addButton.type = 'button';
        addButton.className = 'mcp-button mcp-button-add';
        addButton.textContent = `Add ${schema.title || key}`;
        addButton.addEventListener('click', () => {
            const newItem = this._getDefaultValue(schema.items);
            if (!Array.isArray(this.value[key])) {
                this.value[key] = [];
            }
            this.value[key].push(newItem);
            renderItems();
            this._onChange();
        });

        container.appendChild(addButton);

        return container;
    }

    /**
     * Create array of enum checkboxes
     */
    _createArrayEnumCheckboxes(key, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-array-enum-container';
        
        // Initialize value
        const currentValue = Array.isArray(value) ? [...value] : (schema.default ? [...schema.default] : []);
        this.value[key] = currentValue;
        
        // Create checkboxes for each enum option
        schema.items.enum.forEach(enumValue => {
            const checkboxWrapper = document.createElement('label');
            checkboxWrapper.className = 'mcp-array-enum-item';
            
            const checkbox = document.createElement('input');
            checkbox.type = 'checkbox';
            checkbox.className = 'mcp-checkbox';
            checkbox.value = enumValue;
            checkbox.checked = this.value[key].includes(enumValue);
            
            checkbox.addEventListener('change', () => {
                if (!Array.isArray(this.value[key])) {
                    this.value[key] = [];
                }
                
                if (checkbox.checked) {
                    // Add to array if not already present
                    if (!this.value[key].includes(enumValue)) {
                        this.value[key].push(enumValue);
                    }
                } else {
                    // Remove from array
                    const index = this.value[key].indexOf(enumValue);
                    if (index !== -1) {
                        this.value[key].splice(index, 1);
                    }
                }
                this._onChange();
            });
            
            const label = document.createElement('span');
            label.textContent = enumValue;
            
            checkboxWrapper.appendChild(checkbox);
            checkboxWrapper.appendChild(label);
            container.appendChild(checkboxWrapper);
        });
        
        return container;
    }

    /**
     * Create tuple array input (array of arrays with specific types at each position)
     */
    _createTupleArrayInput(key, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-array-container mcp-tuple-array-container';

        const items = Array.isArray(value) ? value : (schema.default || []);
        this.value[key] = [...items];

        const itemsContainer = document.createElement('div');
        itemsContainer.className = 'mcp-array-items';

        const renderItems = () => {
            itemsContainer.innerHTML = '';
            this.value[key].forEach((item, index) => {
                const itemEl = this._createTupleArrayItem(key, schema.items, item, index);
                itemsContainer.appendChild(itemEl);
            });
        };

        renderItems();
        container.appendChild(itemsContainer);

        // Add button
        const addButton = document.createElement('button');
        addButton.type = 'button';
        addButton.className = 'mcp-button mcp-button-add';
        addButton.textContent = `Add ${schema.title || key}`;
        addButton.addEventListener('click', () => {
            // Create a new tuple with default values for each position
            const newTuple = this._createDefaultTuple(schema.items);
            if (!Array.isArray(this.value[key])) {
                this.value[key] = [];
            }
            this.value[key].push(newTuple);
            renderItems();
            this._onChange();
        });

        container.appendChild(addButton);

        return container;
    }

    /**
     * Create a tuple array item (array with specific types at each position)
     */
    _createTupleArrayItem(arrayKey, tupleSchema, tupleValue, index) {
        const container = document.createElement('div');
        container.className = 'mcp-array-item mcp-tuple-item';

        const fieldsWrapper = document.createElement('div');
        fieldsWrapper.className = 'mcp-tuple-fields';

        // Ensure the tuple value is an array
        if (!Array.isArray(tupleValue)) {
            tupleValue = this._createDefaultTuple(tupleSchema);
            this.value[arrayKey][index] = tupleValue;
        }

        // Handle oneOf items - each position in the tuple has a specific type
        if (tupleSchema.items && tupleSchema.items.oneOf && Array.isArray(tupleSchema.items.oneOf)) {
            tupleSchema.items.oneOf.forEach((positionSchema, positionIndex) => {
                const fieldContainer = document.createElement('div');
                fieldContainer.className = 'mcp-tuple-field';
                
                // Create label if we can determine what this position represents
                const label = document.createElement('label');
                label.className = 'mcp-tuple-field-label';
                if (positionIndex === 0) label.textContent = 'Column';
                else if (positionIndex === 1) label.textContent = 'Operator';
                else if (positionIndex === 2) label.textContent = 'Value';
                else label.textContent = `Field ${positionIndex + 1}`;
                fieldContainer.appendChild(label);

                // Get the value for this position
                const positionValue = tupleValue[positionIndex];
                
                // For the value field with oneOf, create a special multi-type input
                if (positionSchema.oneOf && positionIndex === 2) {
                    const multiTypeContainer = this._createTupleMultiTypeInput(
                        arrayKey, index, positionIndex, positionSchema, positionValue
                    );
                    fieldContainer.appendChild(multiTypeContainer);
                } else {
                    // Create regular input for this position
                    const input = this._createInput(`${arrayKey}[${index}][${positionIndex}]`, positionSchema, positionValue);
                    
                    // We need to handle the value updates ourselves
                    const newInput = input.cloneNode(true);
                    input.parentNode?.replaceChild(newInput, input);
                    
                    // For select elements, we need to set the value after cloning
                    if (newInput.tagName === 'SELECT' && positionValue !== undefined && positionValue !== null) {
                        newInput.value = positionValue;
                    }
                    
                    const updateValue = () => {
                        if (!Array.isArray(this.value[arrayKey])) {
                            this.value[arrayKey] = [];
                        }
                        if (!Array.isArray(this.value[arrayKey][index])) {
                            this.value[arrayKey][index] = [];
                        }
                        
                        let newValue;
                        if (positionSchema.type === 'number' || positionSchema.type === 'integer') {
                            newValue = newInput.value === '' ? undefined : Number(newInput.value);
                        } else if (positionSchema.type === 'boolean') {
                            newValue = newInput.checked;
                        } else {
                            newValue = newInput.value;
                        }
                        
                        this.value[arrayKey][index][positionIndex] = newValue;
                        this._onChange();
                    };

                    if (newInput.tagName === 'INPUT' || newInput.tagName === 'SELECT' || newInput.tagName === 'TEXTAREA') {
                        newInput.addEventListener(newInput.type === 'checkbox' ? 'change' : 'input', updateValue);
                    }

                    fieldContainer.appendChild(newInput);
                }
                
                fieldsWrapper.appendChild(fieldContainer);
            });
        }

        container.appendChild(fieldsWrapper);

        // Remove button
        const removeButton = document.createElement('button');
        removeButton.type = 'button';
        removeButton.className = 'mcp-button mcp-button-remove';
        removeButton.innerHTML = '×';
        removeButton.title = 'Remove';
        removeButton.addEventListener('click', () => {
            if (!Array.isArray(this.value[arrayKey])) {
                this.value[arrayKey] = [];
            }
            this.value[arrayKey].splice(index, 1);
            
            // Re-render the parent array container
            // Find the parent array container and its render function
            const arrayContainer = container.closest('.mcp-tuple-array-container');
            if (arrayContainer) {
                const itemsContainer = arrayContainer.querySelector('.mcp-array-items');
                if (itemsContainer) {
                    // Re-render all items
                    itemsContainer.innerHTML = '';
                    if (Array.isArray(this.value[arrayKey])) {
                        this.value[arrayKey].forEach((item, idx) => {
                            const itemEl = this._createTupleArrayItem(arrayKey, tupleSchema, item, idx);
                            itemsContainer.appendChild(itemEl);
                        });
                    }
                }
            }
            
            this._onChange();
        });
        container.appendChild(removeButton);

        return container;
    }

    /**
     * Create a default tuple based on the schema
     */
    _createDefaultTuple(tupleSchema) {
        if (tupleSchema.items && tupleSchema.items.oneOf && Array.isArray(tupleSchema.items.oneOf)) {
            return tupleSchema.items.oneOf.map(positionSchema => {
                return this._getDefaultValue(positionSchema);
            });
        }
        return [];
    }

    /**
     * Create a multi-type input specifically for tuple arrays
     */
    _createTupleMultiTypeInput(arrayKey, tupleIndex, positionIndex, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-tuple-multitype-container';
        
        // Get the options from oneOf
        const options = schema.oneOf || [];
        
        // Create type selector tabs
        const tabsContainer = document.createElement('div');
        tabsContainer.className = 'mcp-anyof-tabs mcp-tuple-anyof-tabs';
        
        const inputContainer = document.createElement('div');
        inputContainer.className = 'mcp-tuple-multitype-input';
        
        let activeType = 'string';
        let currentInput = null;
        
        // Try to determine the current type based on value
        if (value !== undefined && value !== null) {
            if (typeof value === 'boolean') {
                activeType = 'boolean';
            } else if (typeof value === 'number') {
                activeType = 'number';
            } else {
                activeType = 'string';
            }
        }
        
        // Create tabs for each type
        const tabs = {};
        options.forEach((optionSchema, optionIndex) => {
            const tab = document.createElement('button');
            tab.type = 'button';
            tab.className = 'mcp-anyof-tab';
            tab.textContent = optionSchema.type || 'unknown';
            
            if (optionSchema.type === activeType) {
                tab.classList.add('active');
            }
            
            tab.addEventListener('click', () => {
                // Update active tab
                Object.values(tabs).forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                
                // Create new input for this type
                activeType = optionSchema.type;
                this._updateTupleMultiTypeInput(
                    inputContainer, arrayKey, tupleIndex, positionIndex, 
                    optionSchema, activeType === 'boolean' ? false : (activeType === 'number' ? 0 : '')
                );
            });
            
            tabs[optionSchema.type] = tab;
            tabsContainer.appendChild(tab);
        });
        
        container.appendChild(tabsContainer);
        container.appendChild(inputContainer);
        
        // Create initial input
        const initialSchema = options.find(o => o.type === activeType) || options[0];
        this._updateTupleMultiTypeInput(inputContainer, arrayKey, tupleIndex, positionIndex, initialSchema, value);
        
        return container;
    }
    
    /**
     * Update the input in a tuple multi-type container
     */
    _updateTupleMultiTypeInput(container, arrayKey, tupleIndex, positionIndex, schema, value) {
        container.innerHTML = '';
        
        let input;
        if (schema.type === 'boolean') {
            input = document.createElement('input');
            input.type = 'checkbox';
            input.className = 'mcp-checkbox';
            input.checked = !!value;
        } else if (schema.type === 'number') {
            input = document.createElement('input');
            input.type = 'number';
            input.className = 'mcp-input';
            input.value = value !== undefined ? value : '';
        } else {
            input = document.createElement('input');
            input.type = 'text';
            input.className = 'mcp-input';
            input.value = value || '';
        }
        
        const updateValue = () => {
            if (!Array.isArray(this.value[arrayKey])) {
                this.value[arrayKey] = [];
            }
            if (!Array.isArray(this.value[arrayKey][tupleIndex])) {
                this.value[arrayKey][tupleIndex] = [];
            }
            
            let newValue;
            if (schema.type === 'boolean') {
                newValue = input.checked;
            } else if (schema.type === 'number') {
                newValue = input.value === '' ? undefined : Number(input.value);
            } else {
                newValue = input.value;
            }
            
            this.value[arrayKey][tupleIndex][positionIndex] = newValue;
            this._onChange();
        };
        
        input.addEventListener(schema.type === 'boolean' ? 'change' : 'input', updateValue);
        container.appendChild(input);
    }

    /**
     * Create array item
     */
    _createArrayItem(arrayKey, schema, value, index) {
        const container = document.createElement('div');
        container.className = 'mcp-array-item';

        const inputWrapper = document.createElement('div');
        inputWrapper.className = 'mcp-array-item-input';

        // Create input based on item schema
        const itemSchema = schema.items || { type: 'string' };
        const input = this._createInput(`${arrayKey}[${index}]`, itemSchema, value);
        
        // We need to clear any event listeners that were added by _createInput
        // because they would try to set this.value["dimensions[0]"] instead of this.value["dimensions"][0]
        const newInput = input.cloneNode(true);
        input.parentNode?.replaceChild(newInput, input);
        
        // For select elements, we need to set the value after cloning
        if (newInput.tagName === 'SELECT' && value !== undefined && value !== null) {
            newInput.value = value;
        }
        
        // Add our own handler to update the array properly
        const updateValue = () => {
            if (!Array.isArray(this.value[arrayKey])) {
                this.value[arrayKey] = [];
            }
            if (itemSchema.type === 'number' || itemSchema.type === 'integer') {
                this.value[arrayKey][index] = newInput.value === '' ? undefined : Number(newInput.value);
            } else if (itemSchema.type === 'boolean') {
                this.value[arrayKey][index] = newInput.checked;
            } else {
                this.value[arrayKey][index] = newInput.value;
            }
            this._onChange();
        };

        if (newInput.tagName === 'INPUT' || newInput.tagName === 'SELECT') {
            newInput.addEventListener(newInput.type === 'checkbox' ? 'change' : 'input', updateValue);
        }

        inputWrapper.appendChild(newInput);
        container.appendChild(inputWrapper);

        // Remove button
        const removeButton = document.createElement('button');
        removeButton.type = 'button';
        removeButton.className = 'mcp-button mcp-button-remove';
        removeButton.innerHTML = '×';
        removeButton.title = 'Remove';
        removeButton.addEventListener('click', () => {
            if (!Array.isArray(this.value[arrayKey])) {
                this.value[arrayKey] = [];
            }
            this.value[arrayKey].splice(index, 1);
            const arrayContainer = container.parentElement.parentElement;
            const renderItems = () => {
                const itemsContainer = arrayContainer.querySelector('.mcp-array-items');
                itemsContainer.innerHTML = '';
                this.value[arrayKey].forEach((item, idx) => {
                    const itemEl = this._createArrayItem(arrayKey, schema, item, idx);
                    itemsContainer.appendChild(itemEl);
                });
            };
            renderItems();
            this._onChange();
        });

        container.appendChild(removeButton);

        return container;
    }

    /**
     * Create object input
     */
    _createObjectInput(key, schema, value) {
        // Check if this is a dynamic object with additionalProperties
        if (schema.additionalProperties && !schema.properties) {
            return this._createDynamicObjectInput(key, schema, value);
        }
        
        // For objects with defined properties, create a nested form
        if (schema.properties) {
            return this._createNestedObjectInput(key, schema, value);
        }
        
        // Fallback to JSON editor for unstructured objects
        const textarea = document.createElement('textarea');
        textarea.className = 'mcp-textarea';
        textarea.rows = 4;
        
        try {
            textarea.value = JSON.stringify(value || schema.default || {}, null, 2);
        } catch (e) {
            textarea.value = '{}';
        }

        textarea.addEventListener('input', () => {
            try {
                this.value[key] = JSON.parse(textarea.value);
                textarea.classList.remove('mcp-input-error');
            } catch (e) {
                textarea.classList.add('mcp-input-error');
            }
            this._onChange();
        });

        return textarea;
    }
    
    /**
     * Create dynamic object input for additionalProperties
     */
    _createDynamicObjectInput(key, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-dynamic-object-container';
        
        // Initialize value
        const objectValue = value || {};
        this.value[key] = objectValue;
        
        const itemsContainer = document.createElement('div');
        itemsContainer.className = 'mcp-dynamic-object-items';
        
        // Keep track of items with temporary keys to manage renaming
        let itemIndex = 0;
        const items = new Map();
        
        // Initialize items from existing value
        Object.entries(objectValue).forEach(([k, v]) => {
            items.set(itemIndex++, { key: k, value: v });
        });
        
        const updateData = () => {
            // Rebuild the object from items
            const newObject = {};
            items.forEach((item) => {
                if (item.key) {
                    newObject[item.key] = item.value;
                }
            });
            this.value[key] = newObject;
            this._onChange();
        };
        
        const renderItems = () => {
            itemsContainer.innerHTML = '';
            
            items.forEach((item, idx) => {
                const itemEl = this._createEditableDynamicObjectItem(
                    key, idx, item, schema.additionalProperties, 
                    updateData,  // Just update data, don't re-render
                    items,  // Pass the items Map
                    renderItems  // Pass renderItems separately for when we need full re-render
                );
                itemsContainer.appendChild(itemEl);
            });
        };
        
        renderItems();
        container.appendChild(itemsContainer);
        
        // Add button
        const addButton = document.createElement('button');
        addButton.type = 'button';
        addButton.className = 'mcp-button mcp-button-add';
        addButton.textContent = 'Add Item';
        addButton.addEventListener('click', () => {
            // Add new item with empty key and default value
            const defaultValue = this._getDefaultValue(schema.additionalProperties);
            items.set(itemIndex++, { key: '', value: defaultValue });
            renderItems();
        });
        
        container.appendChild(addButton);
        
        return container;
    }
    
    /**
     * Create an editable dynamic object property item
     */
    _createEditableDynamicObjectItem(parentKey, itemIndex, item, propSchema, updateCallback, itemsMap, rerenderCallback) {
        const container = document.createElement('div');
        container.className = 'mcp-dynamic-object-item';
        
        // Editable key input
        const keyInput = document.createElement('input');
        keyInput.type = 'text';
        keyInput.className = 'mcp-input mcp-dynamic-object-key-input';
        keyInput.value = item.key || '';
        keyInput.placeholder = 'Property name';
        
        keyInput.addEventListener('input', () => {
            const newKey = keyInput.value.trim();
            
            // Update the item's key
            item.key = newKey;
            
            // Don't trigger re-render on every keystroke
        });
        
        keyInput.addEventListener('blur', () => {
            // Trigger update when focus leaves to ensure value is saved
            updateCallback();
        });
        
        container.appendChild(keyInput);
        
        // Property value input
        const valueContainer = document.createElement('div');
        valueContainer.className = 'mcp-dynamic-object-value';
        
        // Create appropriate input based on additionalProperties schema
        if (propSchema.type === 'array') {
            // For array values, create a mini array editor
            const arrayContainer = this._createEditableMiniArrayInput(
                parentKey, itemIndex, item, propSchema.items || {},
                updateCallback,
                false  // Don't re-render parent on array changes
            );
            valueContainer.appendChild(arrayContainer);
        } else {
            // For simple types, create the appropriate input
            const input = this._createInput(`${parentKey}_item_${itemIndex}`, propSchema, item.value);
            if (input.tagName === 'INPUT' || input.tagName === 'SELECT' || input.tagName === 'TEXTAREA') {
                // Remove any existing event listeners from _createInput
                const newInput = input.cloneNode(true);
                input.parentNode?.replaceChild(newInput, input);
                
                const updateValue = () => {
                    if (propSchema.type === 'number' || propSchema.type === 'integer') {
                        item.value = newInput.value === '' ? undefined : Number(newInput.value);
                    } else if (propSchema.type === 'boolean') {
                        item.value = newInput.checked;
                    } else {
                        item.value = newInput.value || undefined;
                    }
                };
                
                if (propSchema.type === 'boolean' || newInput.tagName === 'SELECT') {
                    // For checkbox and select, update immediately
                    newInput.addEventListener('change', () => {
                        updateValue();
                        updateCallback();
                    });
                } else {
                    // For text inputs, update value on input but only re-render on blur
                    newInput.addEventListener('input', updateValue);
                    newInput.addEventListener('blur', () => updateCallback());
                }
                
                valueContainer.appendChild(newInput);
            } else {
                valueContainer.appendChild(input);
            }
        }
        
        container.appendChild(valueContainer);
        
        // Remove button
        const removeButton = document.createElement('button');
        removeButton.type = 'button';
        removeButton.className = 'mcp-button mcp-button-remove';
        removeButton.innerHTML = '×';
        removeButton.title = 'Remove item';
        removeButton.addEventListener('click', () => {
            // Remove from items Map
            if (itemsMap) {
                itemsMap.delete(itemIndex);
            }
            // Need full re-render when removing items
            if (rerenderCallback) {
                rerenderCallback();
            }
            updateCallback();
        });
        container.appendChild(removeButton);
        
        return container;
    }
    
    /**
     * Create editable mini array input for dynamic object properties
     */
    _createEditableMiniArrayInput(parentKey, itemIndex, item, itemSchema, updateCallback, shouldRerenderParent = true) {
        const container = document.createElement('div');
        container.className = 'mcp-mini-array-container';
        
        // Ensure item.value is an array
        if (!Array.isArray(item.value)) {
            item.value = [];
        }
        
        const itemsContainer = document.createElement('div');
        itemsContainer.className = 'mcp-mini-array-items';
        
        // Store references to avoid re-rendering
        const itemElements = new Map();
        
        const addArrayItem = (arrayValue, index) => {
            const itemEl = document.createElement('div');
            itemEl.className = 'mcp-mini-array-item';
            
            // Create input
            const input = this._createInput(`${parentKey}_item_${itemIndex}_arr_${index}`, itemSchema, arrayValue);
            
            if (input.tagName === 'INPUT' || input.tagName === 'SELECT' || input.tagName === 'TEXTAREA') {
                // Remove any existing event listeners from _createInput
                const newInput = input.cloneNode(true);
                
                const updateValue = () => {
                    if (itemSchema.type === 'number' || itemSchema.type === 'integer') {
                        item.value[index] = newInput.value === '' ? undefined : Number(newInput.value);
                    } else if (itemSchema.type === 'boolean') {
                        item.value[index] = newInput.checked;
                    } else {
                        item.value[index] = newInput.value || undefined;
                    }
                };
                
                if (itemSchema.type === 'boolean' || newInput.tagName === 'SELECT') {
                    newInput.addEventListener('change', () => {
                        updateValue();
                        this._onChange();
                    });
                } else {
                    newInput.addEventListener('input', updateValue);
                    newInput.addEventListener('blur', () => this._onChange());
                }
                
                itemEl.appendChild(newInput);
            } else {
                itemEl.appendChild(input);
            }
            
            // Remove button
            const removeButton = document.createElement('button');
            removeButton.type = 'button';
            removeButton.className = 'mcp-button mcp-button-remove-small';
            removeButton.innerHTML = '×';
            removeButton.title = 'Remove item';
            removeButton.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                
                // Remove from array
                item.value.splice(index, 1);
                
                // Remove DOM element
                itemEl.remove();
                
                // Update indices for remaining items
                itemElements.delete(index);
                const newElements = new Map();
                let newIndex = 0;
                itemElements.forEach((el, oldIndex) => {
                    if (oldIndex > index) {
                        newElements.set(newIndex, el);
                        // Update the input name attribute if needed
                        const input = el.querySelector('input, select, textarea');
                        if (input) {
                            input.name = `${parentKey}_item_${itemIndex}_arr_${newIndex}`;
                        }
                    } else if (oldIndex < index) {
                        newElements.set(newIndex, el);
                    }
                    newIndex++;
                });
                itemElements.clear();
                newElements.forEach((el, idx) => itemElements.set(idx, el));
                
                this._onChange();
            });
            
            itemEl.appendChild(removeButton);
            itemsContainer.appendChild(itemEl);
            itemElements.set(index, itemEl);
        };
        
        // Initial render
        item.value.forEach((arrayValue, index) => {
            addArrayItem(arrayValue, index);
        });
        
        container.appendChild(itemsContainer);
        
        // Add button
        const addButton = document.createElement('button');
        addButton.type = 'button';
        addButton.className = 'mcp-button mcp-button-add-small';
        addButton.textContent = '+ Add';
        addButton.addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            
            const newItem = this._getDefaultValue(itemSchema);
            const newIndex = item.value.length;
            item.value.push(newItem);
            
            // Add DOM element without re-rendering everything
            addArrayItem(newItem, newIndex);
            
            this._onChange();
        });
        container.appendChild(addButton);
        
        return container;
    }
    
    /**
     * Create editable mini array item for dynamic object properties
     */
    _createEditableMiniArrayItem(parentKey, itemIndex, item, arrayItem, index, itemSchema, updateCallback) {
        const container = document.createElement('div');
        container.className = 'mcp-mini-array-item';
        
        // Create input based on schema
        const input = this._createInput(`${parentKey}_item_${itemIndex}_arr_${index}`, itemSchema, arrayItem);
        
        if (input.tagName === 'INPUT' || input.tagName === 'SELECT' || input.tagName === 'TEXTAREA') {
            // Remove any existing event listeners from _createInput
            const newInput = input.cloneNode(true);
            input.parentNode?.replaceChild(newInput, input);
            
            const updateValue = () => {
                // Update the array item value
                if (!Array.isArray(item.value)) {
                    item.value = [];
                }
                
                if (itemSchema.type === 'number' || itemSchema.type === 'integer') {
                    item.value[index] = newInput.value === '' ? undefined : Number(newInput.value);
                } else if (itemSchema.type === 'boolean') {
                    item.value[index] = newInput.checked;
                } else {
                    item.value[index] = newInput.value || undefined;
                }
            };
            
            if (itemSchema.type === 'boolean' || newInput.tagName === 'SELECT') {
                // For checkbox and select, update immediately
                newInput.addEventListener('change', () => {
                    updateValue();
                    updateCallback();
                });
            } else {
                // For text inputs, update value on input but only re-render on blur
                newInput.addEventListener('input', updateValue);
                newInput.addEventListener('blur', () => updateCallback());
            }
            
            container.appendChild(newInput);
        } else {
            container.appendChild(input);
        }
        
        // Remove button
        const removeButton = document.createElement('button');
        removeButton.type = 'button';
        removeButton.className = 'mcp-button mcp-button-remove-small';
        removeButton.innerHTML = '×';
        removeButton.title = 'Remove item';
        removeButton.addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            if (Array.isArray(item.value)) {
                item.value.splice(index, 1);
                // Just trigger local re-render, not parent update
                updateCallback();
            }
        });
        container.appendChild(removeButton);
        
        return container;
    }
    
    /**
     * Create mini array item
     */
    _createMiniArrayItem(parentKey, propKey, value, index, schema, renderCallback) {
        const container = document.createElement('div');
        container.className = 'mcp-mini-array-item';
        
        // Create input based on schema
        const input = this._createInput(`${parentKey}_${propKey}_${index}`, schema, value);
        
        if (input.tagName === 'INPUT' || input.tagName === 'SELECT' || input.tagName === 'TEXTAREA') {
            input.addEventListener('input', () => {
                if (schema.type === 'number' || schema.type === 'integer') {
                    this.value[parentKey][propKey][index] = input.value === '' ? undefined : Number(input.value);
                } else if (schema.type === 'boolean') {
                    this.value[parentKey][propKey][index] = input.checked;
                } else {
                    this.value[parentKey][propKey][index] = input.value || undefined;
                }
                this._onChange();
            });
        }
        
        container.appendChild(input);
        
        // Remove button
        const removeButton = document.createElement('button');
        removeButton.type = 'button';
        removeButton.className = 'mcp-button mcp-button-remove-small';
        removeButton.innerHTML = '×';
        removeButton.addEventListener('click', () => {
            if (this.value[parentKey] && Array.isArray(this.value[parentKey][propKey])) {
                this.value[parentKey][propKey].splice(index, 1);
                renderCallback();
                this._onChange();
            }
        });
        container.appendChild(removeButton);
        
        return container;
    }
    
    /**
     * Create nested object input for objects with defined properties
     */
    _createNestedObjectInput(key, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-nested-object';
        
        // Initialize object value
        this.value[key] = value || {};
        
        // Render each property
        for (const [propKey, propSchema] of Object.entries(schema.properties)) {
            const fieldContainer = this._renderField(`${key}.${propKey}`, propSchema, this.value[key][propKey]);
            
            // Update parent object when nested field changes
            const originalOnChange = this.options.onChange;
            this.options.onChange = () => {
                this.value[key][propKey] = this.value[`${key}.${propKey}`];
                originalOnChange(this.getValue());
            };
            
            container.appendChild(fieldContainer);
        }
        
        return container;
    }

    /**
     * Create multi-type input for anyOf/oneOf
     */
    _createMultiTypeInput(key, schema, value) {
        const container = document.createElement('div');
        container.className = 'mcp-anyof-container';
        
        // Get the options
        const options = schema.anyOf || schema.oneOf || [];
        
        // Create tabs
        const tabsContainer = document.createElement('div');
        tabsContainer.className = 'mcp-anyof-tabs';
        
        const contentContainer = document.createElement('div');
        contentContainer.className = 'mcp-anyof-content';
        
        let activeIndex = 0;
        
        // Try to determine which schema matches the current value
        if (value !== undefined && value !== null) {
            options.forEach((opt, index) => {
                if (this._valueMatchesSchema(value, opt)) {
                    activeIndex = index;
                }
            });
        }
        
        const contents = [];
        
        options.forEach((optionSchema, index) => {
            // Create tab
            const tab = document.createElement('div');
            tab.className = 'mcp-anyof-tab' + (index === activeIndex ? ' active' : '');
            
            // Determine tab label
            let label = optionSchema.title || '';
            if (!label) {
                if (optionSchema.type) {
                    label = optionSchema.type;
                    // Add more context for certain types
                    if (optionSchema.type === 'string' && optionSchema.format) {
                        label += ` (${optionSchema.format})`;
                    } else if (optionSchema.type === 'number' && optionSchema.description) {
                        // Try to extract key info from description
                        if (optionSchema.description.includes('timestamp')) {
                            label = 'timestamp';
                        } else if (optionSchema.description.includes('relative')) {
                            label = 'relative';
                        }
                    }
                } else if (optionSchema.enum) {
                    label = 'enum';
                } else {
                    label = `Option ${index + 1}`;
                }
            }
            tab.textContent = label;
            
            // Create content
            const content = document.createElement('div');
            content.className = 'mcp-anyof-panel' + (index === activeIndex ? ' active' : '');
            
            // If the option has a description, show it
            if (optionSchema.description) {
                const desc = document.createElement('div');
                desc.className = 'mcp-anyof-description';
                desc.innerHTML = optionSchema.description.replace(/\n/g, '<br/>');
                content.appendChild(desc);
            }
            
            // Create the input for this option
            const optionInput = this._createInput(key + '_anyof_' + index, optionSchema, value);
            content.appendChild(optionInput);
            
            // Handle value updates based on the input type
            if (optionInput.tagName === 'INPUT' || optionInput.tagName === 'SELECT' || optionInput.tagName === 'TEXTAREA') {
                const updateValue = () => {
                    if (optionSchema.type === 'number' || optionSchema.type === 'integer') {
                        this.value[key] = optionInput.value === '' ? undefined : Number(optionInput.value);
                    } else if (optionSchema.type === 'boolean') {
                        this.value[key] = optionInput.checked;
                    } else {
                        this.value[key] = optionInput.value || undefined;
                    }
                    this._onChange();
                };
                
                optionInput.addEventListener(optionInput.type === 'checkbox' ? 'change' : 'input', updateValue);
            } else if (optionSchema.type === 'array' || optionSchema.type === 'object') {
                // For complex types, we need to handle updates differently
                // The array/object inputs handle their own value updates
                // We just need to ensure the parent value is properly set
                this.value[key] = this.value[key] || this._getDefaultValue(optionSchema);
            }
            
            // Tab click handler
            tab.addEventListener('click', () => {
                // Update active states
                container.querySelectorAll('.mcp-anyof-tab').forEach(t => t.classList.remove('active'));
                container.querySelectorAll('.mcp-anyof-panel').forEach(p => p.classList.remove('active'));
                tab.classList.add('active');
                content.classList.add('active');
                
                // Update the value based on the selected type
                const input = content.querySelector('input, select, textarea');
                if (input && input.value) {
                    input.dispatchEvent(new Event('input'));
                }
            });
            
            tabsContainer.appendChild(tab);
            contentContainer.appendChild(content);
            contents.push({ tab, content, input: optionInput });
        });
        
        container.appendChild(tabsContainer);
        container.appendChild(contentContainer);
        
        return container;
    }
    
    /**
     * Check if a value matches a schema type
     */
    _valueMatchesSchema(value, schema) {
        if (schema.type) {
            switch (schema.type) {
                case 'string':
                    return typeof value === 'string';
                case 'number':
                case 'integer':
                    return typeof value === 'number';
                case 'boolean':
                    return typeof value === 'boolean';
                case 'array':
                    return Array.isArray(value);
                case 'object':
                    return typeof value === 'object' && value !== null && !Array.isArray(value);
            }
        }
        if (schema.enum && schema.enum.includes(value)) {
            return true;
        }
        return false;
    }

    /**
     * Get default value for a schema
     */
    _getDefaultValue(schema) {
        if (schema.default !== undefined) return schema.default;
        
        switch (schema.type) {
            case 'string': return '';
            case 'number':
            case 'integer': return 0;
            case 'boolean': return false;
            case 'array': return [];
            case 'object': return {};
            default: return null;
        }
    }

    /**
     * Validate a field value
     */
    _validateField(value, schema) {
        if (schema.enum && !schema.enum.includes(value)) {
            return `Value must be one of: ${schema.enum.join(', ')}`;
        }

        switch (schema.type) {
            case 'string':
                if (schema.minLength && (!value || value.length < schema.minLength)) {
                    return `Minimum length is ${schema.minLength}`;
                }
                if (schema.maxLength && value && value.length > schema.maxLength) {
                    return `Maximum length is ${schema.maxLength}`;
                }
                if (schema.pattern && value && !new RegExp(schema.pattern).test(value)) {
                    return 'Invalid format';
                }
                break;
            case 'number':
            case 'integer':
                if (value === undefined || value === null || value === '') return null;
                if (schema.minimum !== undefined && value < schema.minimum) {
                    return `Minimum value is ${schema.minimum}`;
                }
                if (schema.maximum !== undefined && value > schema.maximum) {
                    return `Maximum value is ${schema.maximum}`;
                }
                if (schema.type === 'integer' && !Number.isInteger(value)) {
                    return 'Must be an integer';
                }
                break;
        }

        return null;
    }

    /**
     * Update error display
     */
    _updateErrorDisplay() {
        for (const [field, container] of this.fieldElements) {
            const errorEl = container.querySelector('.mcp-field-error');
            if (this.errors[field]) {
                errorEl.textContent = this.errors[field];
                errorEl.style.display = 'block';
                container.classList.add('has-error');
            } else {
                errorEl.textContent = '';
                errorEl.style.display = 'none';
                container.classList.remove('has-error');
            }
        }
    }
    
    /**
     * Clear all validation errors
     */
    clearErrors() {
        this.errors = {};
        this._updateErrorDisplay();
    }

    /**
     * Update field values from current value
     */
    _updateFieldValues() {
        // Re-render the entire form with the new values
        // This is simpler than trying to update individual fields
        if (this.schema) {
            this.render();
        }
    }

    /**
     * Handle value change
     */
    _onChange() {
        // Debounce changes to avoid rapid re-renders
        if (this._changeTimer) {
            clearTimeout(this._changeTimer);
        }
        
        this._changeTimer = setTimeout(() => {
            // Don't validate on every change - only validate when explicitly requested
            this.options.onChange(this.getValue());
        }, 50);  // 50ms debounce
    }

    /**
     * Validate that the schema conforms to MCP specification
     */
    _validateMCPSchema(schema) {
        const errors = [];

        // Root schema validation
        if (!schema || typeof schema !== 'object') {
            throw new Error('MCP Schema Validation Error: Schema must be an object');
        }

        // Must have type: "object"
        if (schema.type !== 'object') {
            errors.push('Root schema must have type: "object"');
        }

        // Must have "type" property
        if (!schema.hasOwnProperty('type')) {
            errors.push('Root schema must have "type" property');
        }

        // Validate properties
        if (schema.properties) {
            if (typeof schema.properties !== 'object') {
                errors.push('Properties must be an object');
            } else {
                for (const [propName, propSchema] of Object.entries(schema.properties)) {
                    const propErrors = this._validateProperty(propName, propSchema, `properties.${propName}`);
                    errors.push(...propErrors);
                }
            }
        }

        // Validate required array
        if (schema.required !== undefined) {
            if (!Array.isArray(schema.required)) {
                errors.push('Required must be an array');
            } else {
                for (const requiredProp of schema.required) {
                    if (typeof requiredProp !== 'string') {
                        errors.push(`Required property "${requiredProp}" must be a string`);
                    }
                    if (schema.properties && !schema.properties.hasOwnProperty(requiredProp)) {
                        errors.push(`Required property "${requiredProp}" is not defined in properties`);
                    }
                }
            }
        }

        // Check for allowed root properties
        const allowedRootProps = ['type', 'properties', 'required', 'title', 'description', 'additionalProperties', '$schema'];
        for (const prop of Object.keys(schema)) {
            if (!allowedRootProps.includes(prop)) {
                errors.push(`Unknown root property: "${prop}"`);
            }
        }

        if (errors.length > 0) {
            throw new Error(`MCP Schema Validation Errors:\n${errors.map(e => `- ${e}`).join('\n')}`);
        }
    }

    /**
     * Validate a property schema
     */
    _validateProperty(propName, propSchema, path) {
        const errors = [];

        if (!propSchema || typeof propSchema !== 'object') {
            errors.push(`${path}: Property schema must be an object`);
            return errors;
        }

        // Must have a type or be anyOf/oneOf
        if (!propSchema.type && !propSchema.anyOf && !propSchema.oneOf && !propSchema.enum) {
            errors.push(`${path}: Property must have "type", "anyOf", "oneOf", or "enum"`);
        }

        // Validate type
        if (propSchema.type) {
            const validTypes = ['string', 'number', 'integer', 'boolean', 'array', 'object', 'null'];
            if (!validTypes.includes(propSchema.type)) {
                errors.push(`${path}: Invalid type "${propSchema.type}". Must be one of: ${validTypes.join(', ')}`);
            }
        }

        // Type-specific validation
        if (propSchema.type === 'array') {
            if (propSchema.items) {
                if (typeof propSchema.items !== 'object') {
                    errors.push(`${path}.items: Array items must be an object schema`);
                } else {
                    const itemErrors = this._validateProperty(`${propName}[]`, propSchema.items, `${path}.items`);
                    errors.push(...itemErrors);
                }
            }
        }

        if (propSchema.type === 'object') {
            if (propSchema.properties) {
                for (const [subPropName, subPropSchema] of Object.entries(propSchema.properties)) {
                    const subErrors = this._validateProperty(subPropName, subPropSchema, `${path}.properties.${subPropName}`);
                    errors.push(...subErrors);
                }
            }
        }

        // Validate enum
        if (propSchema.enum) {
            if (!Array.isArray(propSchema.enum)) {
                errors.push(`${path}.enum: Enum must be an array`);
            } else if (propSchema.enum.length === 0) {
                errors.push(`${path}.enum: Enum array cannot be empty`);
            }
        }

        // Validate anyOf/oneOf
        if (propSchema.anyOf) {
            if (!Array.isArray(propSchema.anyOf)) {
                errors.push(`${path}.anyOf: anyOf must be an array`);
            } else {
                propSchema.anyOf.forEach((subSchema, index) => {
                    const subErrors = this._validateProperty(`${propName}[${index}]`, subSchema, `${path}.anyOf[${index}]`);
                    errors.push(...subErrors);
                });
            }
        }

        if (propSchema.oneOf) {
            if (!Array.isArray(propSchema.oneOf)) {
                errors.push(`${path}.oneOf: oneOf must be an array`);
            } else {
                propSchema.oneOf.forEach((subSchema, index) => {
                    const subErrors = this._validateProperty(`${propName}[${index}]`, subSchema, `${path}.oneOf[${index}]`);
                    errors.push(...subErrors);
                });
            }
        }

        // Check for allowed property keywords
        const allowedProps = [
            // Core keywords
            'type', 'enum', 'const', 
            // String keywords
            'minLength', 'maxLength', 'pattern', 'format',
            // Number keywords
            'minimum', 'maximum', 'exclusiveMinimum', 'exclusiveMaximum', 'multipleOf',
            // Array keywords
            'items', 'minItems', 'maxItems', 'uniqueItems',
            // Object keywords
            'properties', 'required', 'additionalProperties', 'patternProperties',
            // Composition keywords
            'anyOf', 'oneOf', 'allOf', 'not',
            // Metadata keywords
            'title', 'description', 'default', 'examples',
            // Additional validation
            'if', 'then', 'else'
        ];

        for (const prop of Object.keys(propSchema)) {
            if (!allowedProps.includes(prop)) {
                errors.push(`${path}: Unknown property keyword: "${prop}"`);
            }
        }

        // Validate default value matches type
        if (propSchema.default !== undefined) {
            const defaultError = this._validateDefaultValue(propSchema.default, propSchema, path);
            if (defaultError) {
                errors.push(defaultError);
            }
        }

        // Validate string constraints
        if (propSchema.type === 'string') {
            if (propSchema.minLength !== undefined && (typeof propSchema.minLength !== 'number' || propSchema.minLength < 0)) {
                errors.push(`${path}.minLength: Must be a non-negative number`);
            }
            if (propSchema.maxLength !== undefined && (typeof propSchema.maxLength !== 'number' || propSchema.maxLength < 0)) {
                errors.push(`${path}.maxLength: Must be a non-negative number`);
            }
            if (propSchema.pattern !== undefined && typeof propSchema.pattern !== 'string') {
                errors.push(`${path}.pattern: Must be a string`);
            }
        }

        // Validate number constraints
        if (propSchema.type === 'number' || propSchema.type === 'integer') {
            if (propSchema.minimum !== undefined && typeof propSchema.minimum !== 'number') {
                errors.push(`${path}.minimum: Must be a number`);
            }
            if (propSchema.maximum !== undefined && typeof propSchema.maximum !== 'number') {
                errors.push(`${path}.maximum: Must be a number`);
            }
            if (propSchema.multipleOf !== undefined && (typeof propSchema.multipleOf !== 'number' || propSchema.multipleOf <= 0)) {
                errors.push(`${path}.multipleOf: Must be a positive number`);
            }
        }

        // Validate array constraints
        if (propSchema.type === 'array') {
            if (propSchema.minItems !== undefined && (typeof propSchema.minItems !== 'number' || propSchema.minItems < 0)) {
                errors.push(`${path}.minItems: Must be a non-negative number`);
            }
            if (propSchema.maxItems !== undefined && (typeof propSchema.maxItems !== 'number' || propSchema.maxItems < 0)) {
                errors.push(`${path}.maxItems: Must be a non-negative number`);
            }
        }

        return errors;
    }

    /**
     * Validate that a default value matches the schema type
     */
    _validateDefaultValue(defaultValue, schema, path) {
        // Handle null
        if (defaultValue === null) {
            if (schema.type === 'null' || (Array.isArray(schema.type) && schema.type.includes('null'))) {
                return null;
            }
            return `${path}.default: Default value null is not valid for type ${schema.type}`;
        }

        // Handle enum
        if (schema.enum) {
            if (!schema.enum.includes(defaultValue)) {
                return `${path}.default: Default value must be one of enum values: ${schema.enum.join(', ')}`;
            }
            return null;
        }

        // Validate based on type
        switch (schema.type) {
            case 'string':
                if (typeof defaultValue !== 'string') {
                    return `${path}.default: Default value must be a string, got ${typeof defaultValue}`;
                }
                // Check string constraints
                if (schema.minLength !== undefined && defaultValue.length < schema.minLength) {
                    return `${path}.default: Default value length ${defaultValue.length} is less than minLength ${schema.minLength}`;
                }
                if (schema.maxLength !== undefined && defaultValue.length > schema.maxLength) {
                    return `${path}.default: Default value length ${defaultValue.length} exceeds maxLength ${schema.maxLength}`;
                }
                if (schema.pattern && !new RegExp(schema.pattern).test(defaultValue)) {
                    return `${path}.default: Default value does not match pattern ${schema.pattern}`;
                }
                break;

            case 'number':
                if (typeof defaultValue !== 'number' || isNaN(defaultValue)) {
                    return `${path}.default: Default value must be a number, got ${typeof defaultValue}`;
                }
                // Check number constraints
                if (schema.minimum !== undefined && defaultValue < schema.minimum) {
                    return `${path}.default: Default value ${defaultValue} is less than minimum ${schema.minimum}`;
                }
                if (schema.maximum !== undefined && defaultValue > schema.maximum) {
                    return `${path}.default: Default value ${defaultValue} exceeds maximum ${schema.maximum}`;
                }
                if (schema.multipleOf !== undefined && defaultValue % schema.multipleOf !== 0) {
                    return `${path}.default: Default value ${defaultValue} is not a multiple of ${schema.multipleOf}`;
                }
                break;

            case 'integer':
                if (typeof defaultValue !== 'number' || !Number.isInteger(defaultValue)) {
                    return `${path}.default: Default value must be an integer, got ${typeof defaultValue}`;
                }
                // Check integer constraints (same as number)
                if (schema.minimum !== undefined && defaultValue < schema.minimum) {
                    return `${path}.default: Default value ${defaultValue} is less than minimum ${schema.minimum}`;
                }
                if (schema.maximum !== undefined && defaultValue > schema.maximum) {
                    return `${path}.default: Default value ${defaultValue} exceeds maximum ${schema.maximum}`;
                }
                break;

            case 'boolean':
                if (typeof defaultValue !== 'boolean') {
                    return `${path}.default: Default value must be a boolean, got ${typeof defaultValue}`;
                }
                break;

            case 'array':
                if (!Array.isArray(defaultValue)) {
                    return `${path}.default: Default value must be an array, got ${typeof defaultValue}`;
                }
                // Check array constraints
                if (schema.minItems !== undefined && defaultValue.length < schema.minItems) {
                    return `${path}.default: Default array length ${defaultValue.length} is less than minItems ${schema.minItems}`;
                }
                if (schema.maxItems !== undefined && defaultValue.length > schema.maxItems) {
                    return `${path}.default: Default array length ${defaultValue.length} exceeds maxItems ${schema.maxItems}`;
                }
                // Could validate items too, but that might be too complex for now
                break;

            case 'object':
                if (typeof defaultValue !== 'object' || defaultValue === null || Array.isArray(defaultValue)) {
                    return `${path}.default: Default value must be an object, got ${typeof defaultValue}`;
                }
                // Could validate properties too, but that might be too complex for now
                break;

            case 'null':
                if (defaultValue !== null) {
                    return `${path}.default: Default value must be null, got ${typeof defaultValue}`;
                }
                break;
        }

        return null;
    }

    /**
     * Deep clone an object
     */
    _deepClone(obj) {
        return JSON.parse(JSON.stringify(obj));
    }

    /**
     * Inject CSS styles
     */
    _injectStyles() {
        if (document.getElementById('mcp-schema-ui-styles')) return;

        const style = document.createElement('style');
        style.id = 'mcp-schema-ui-styles';
        style.textContent = `
            .mcp-form {
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                font-size: 14px;
                line-height: 1.5;
            }

            .mcp-form-title {
                margin: 0 0 12px 0;
                font-size: 16px;
                font-weight: 600;
                color: #333;
            }

            .mcp-form-description {
                margin: 0 0 16px 0;
                color: #666;
                font-size: 13px;
            }

            .mcp-field {
                margin-bottom: 16px;
            }

            .mcp-field-label-wrapper {
                display: flex;
                align-items: center;
                gap: 6px;
                margin-bottom: 6px;
                justify-content: space-between;
            }

            .mcp-field-label {
                font-weight: 500;
                color: #333;
                display: flex;
                align-items: center;
                gap: 4px;
            }

            .mcp-field-label-hoverable {
                cursor: help;
                position: relative;
            }

            .mcp-field-label-hoverable:hover {
                color: #0088cc;
                text-decoration: underline;
                text-decoration-style: dotted;
                text-underline-offset: 2px;
            }

            .mcp-field-required {
                color: #dc3545;
                font-weight: normal;
            }

            .mcp-field-json-name {
                font-family: 'Courier New', monospace;
                font-size: 12px;
                color: #6c757d;
                margin-left: auto;
            }

            .mcp-tooltip-popup {
                background: rgba(0, 0, 0, 0.9);
                color: white;
                padding: 8px 12px;
                border-radius: 4px;
                font-size: 12px;
                line-height: 1.4;
                white-space: pre-wrap;
                max-width: 300px;
                pointer-events: none;
                z-index: 10000;
                box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
                animation: mcp-tooltip-fade-in 0.2s ease-out;
            }

            @keyframes mcp-tooltip-fade-in {
                from {
                    opacity: 0;
                    transform: translateY(-95%);
                }
                to {
                    opacity: 1;
                    transform: translateY(-100%);
                }
            }

            .mcp-field-description {
                font-size: 12px;
                color: #6c757d;
                margin-bottom: 6px;
            }

            .mcp-field-error {
                color: #dc3545;
                font-size: 12px;
                margin-top: 4px;
                display: none;
            }

            .mcp-field.has-error .mcp-input,
            .mcp-field.has-error .mcp-select,
            .mcp-field.has-error .mcp-textarea {
                border-color: #dc3545;
            }

            .mcp-input,
            .mcp-select,
            .mcp-textarea {
                width: 100%;
                padding: 6px 10px;
                border: 1px solid #ced4da;
                border-radius: 4px;
                font-size: 14px;
                transition: border-color 0.15s ease-in-out;
            }

            .mcp-input:focus,
            .mcp-select:focus,
            .mcp-textarea:focus {
                outline: none;
                border-color: #80bdff;
                box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.1);
            }

            .mcp-input-error {
                border-color: #dc3545 !important;
            }

            .mcp-checkbox-wrapper {
                display: flex;
                align-items: center;
                gap: 8px;
                cursor: pointer;
            }

            .mcp-checkbox {
                width: 16px;
                height: 16px;
                cursor: pointer;
            }

            .mcp-array-container {
                border: 1px solid #dee2e6;
                border-radius: 4px;
                padding: 12px;
                background: #f8f9fa;
            }

            .mcp-array-items {
                margin-bottom: 8px;
            }

            .mcp-array-item {
                display: flex;
                gap: 8px;
                margin-bottom: 8px;
                align-items: flex-start;
            }

            .mcp-array-item-input {
                flex: 1;
            }

            .mcp-button {
                padding: 6px 12px;
                border: 1px solid #ced4da;
                border-radius: 4px;
                background: white;
                font-size: 13px;
                cursor: pointer;
                transition: all 0.15s ease;
            }

            .mcp-button:hover {
                background: #f8f9fa;
                border-color: #adb5bd;
            }

            .mcp-button-add {
                color: #28a745;
                border-color: #28a745;
            }

            .mcp-button-add:hover {
                background: #28a745;
                color: white;
            }

            .mcp-button-remove {
                width: 28px;
                height: 28px;
                padding: 0;
                color: #dc3545;
                border-color: #dc3545;
                font-size: 18px;
                line-height: 1;
            }

            .mcp-button-remove:hover {
                background: #dc3545;
                color: white;
            }

            /* AnyOf/OneOf tabs */
            .mcp-anyof-container {
                border: 1px solid #dee2e6;
                border-radius: 4px;
                overflow: hidden;
            }

            .mcp-anyof-tabs {
                display: flex;
                background: #f8f9fa;
                border-bottom: 1px solid #dee2e6;
            }

            .mcp-anyof-tab {
                padding: 8px 16px;
                cursor: pointer;
                font-size: 13px;
                color: #666;
                border-right: 1px solid #dee2e6;
                transition: all 0.2s;
                background: #f8f9fa;
            }

            .mcp-anyof-tab:last-child {
                border-right: none;
            }

            .mcp-anyof-tab:hover {
                background: #e9ecef;
                color: #333;
            }

            .mcp-anyof-tab.active {
                background: white;
                color: #0088cc;
                font-weight: 600;
                border-bottom: 2px solid white;
                margin-bottom: -1px;
            }

            .mcp-anyof-content {
                background: white;
                min-height: 60px;
            }

            .mcp-anyof-panel {
                display: none;
                padding: 12px;
            }

            .mcp-anyof-panel.active {
                display: block;
            }

            .mcp-anyof-description {
                font-size: 12px;
                color: #666;
                margin-bottom: 10px;
                line-height: 1.4;
                background: #f8f9fa;
                padding: 8px 10px;
                border-radius: 4px;
            }

            /* Dynamic object styles */
            .mcp-dynamic-object-container {
                border: 1px solid #dee2e6;
                border-radius: 4px;
                padding: 8px;
                background: #f8f9fa;
            }

            .mcp-dynamic-object-items {
                margin-bottom: 8px;
            }

            .mcp-dynamic-object-item {
                display: grid;
                grid-template-columns: 150px 1fr auto;
                gap: 8px;
                align-items: start;
                padding: 8px;
                background: white;
                border: 1px solid #dee2e6;
                border-radius: 4px;
                margin-bottom: 6px;
            }

            .mcp-dynamic-object-key {
                font-weight: 600;
                color: #333;
                padding: 6px 0;
                word-break: break-word;
            }

            .mcp-dynamic-object-value {
                min-width: 0;
            }

            .mcp-dynamic-object-add {
                display: flex;
                gap: 8px;
                padding: 8px;
                background: white;
                border: 1px solid #dee2e6;
                border-radius: 4px;
            }

            .mcp-dynamic-key-input {
                flex: 1;
                max-width: 200px;
            }

            /* Mini array styles for dynamic objects */
            .mcp-mini-array-container {
                background: #f8f9fa;
                padding: 6px;
                border-radius: 4px;
                border: 1px solid #e9ecef;
            }

            .mcp-mini-array-items {
                display: flex;
                flex-direction: column;
                gap: 4px;
                margin-bottom: 6px;
            }

            .mcp-mini-array-item {
                display: flex;
                gap: 6px;
                align-items: center;
            }

            .mcp-mini-array-item input,
            .mcp-mini-array-item select {
                flex: 1;
                min-width: 0;
            }

            .mcp-button-add-small,
            .mcp-button-remove-small {
                padding: 2px 8px;
                font-size: 12px;
                line-height: 1.5;
                white-space: nowrap;
                min-width: fit-content;
            }
            
            .mcp-button-add-small {
                color: #28a745;
                border-color: #28a745;
            }
            
            .mcp-button-add-small:hover {
                background: #28a745;
                color: white;
            }

            .mcp-button-remove-small {
                width: 24px;
                height: 24px;
                padding: 0;
                color: #dc3545;
                border-color: #dc3545;
            }

            .mcp-button-remove-small:hover {
                background: #dc3545;
                color: white;
            }

            /* Array enum checkbox styles */
            .mcp-array-enum-container {
                display: flex;
                flex-direction: column;
                gap: 8px;
                padding: 8px;
                background: #f8f9fa;
                border: 1px solid #dee2e6;
                border-radius: 4px;
            }

            .mcp-array-enum-item {
                display: flex;
                align-items: center;
                gap: 8px;
                cursor: pointer;
                padding: 4px 0;
            }

            .mcp-array-enum-item:hover {
                background: rgba(0, 123, 255, 0.05);
                border-radius: 3px;
                margin: 0 -4px;
                padding: 4px;
            }

            .mcp-array-enum-item input[type="checkbox"] {
                width: 16px;
                height: 16px;
                cursor: pointer;
                margin: 0;
            }

            .mcp-array-enum-item span {
                user-select: none;
                color: #333;
                font-size: 14px;
            }

            /* Nested object styles */
            .mcp-nested-object {
                padding: 12px;
                background: #f8f9fa;
                border: 1px solid #dee2e6;
                border-radius: 4px;
            }

            .mcp-nested-object .mcp-field {
                margin-bottom: 12px;
            }

            .mcp-nested-object .mcp-field:last-child {
                margin-bottom: 0;
            }

            /* Tuple array styles */
            .mcp-tuple-array-container {
                background: #f0f0f0;
            }

            .mcp-tuple-item {
                background: white;
                border: 1px solid #ddd;
                border-radius: 4px;
                padding: 8px;
            }

            .mcp-tuple-fields {
                display: flex;
                gap: 12px;
                align-items: flex-start;
                flex: 1;
            }

            .mcp-tuple-field {
                flex: 1;
                min-width: 0;
            }

            .mcp-tuple-field:first-child {
                flex: 1.6; /* Column name - 20% narrower than before */
            }

            .mcp-tuple-field:nth-child(2) {
                flex: 0.75; /* Operator - half width */
            }

            .mcp-tuple-field:nth-child(3) {
                flex: 2; /* Value */
            }

            .mcp-tuple-field-label {
                font-size: 11px;
                color: #666;
                margin-bottom: 4px;
                font-weight: 500;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .mcp-tuple-field input,
            .mcp-tuple-field select {
                width: 100%;
                box-sizing: border-box;
            }

            .mcp-array-item.mcp-tuple-item {
                display: flex;
                gap: 8px;
                align-items: center;
            }

            .mcp-array-item.mcp-tuple-item .mcp-button-remove {
                flex-shrink: 0;
                align-self: flex-start;
                margin-top: 20px; /* Align with inputs */
            }

            /* Tuple multi-type input styles */
            .mcp-tuple-multitype-container {
                display: flex;
                flex-direction: column;
                gap: 4px;
            }

            .mcp-tuple-anyof-tabs {
                display: flex;
                gap: 2px;
                background: #f0f0f0;
                padding: 2px;
                border-radius: 4px;
            }

            .mcp-tuple-anyof-tabs .mcp-anyof-tab {
                padding: 2px 8px;
                font-size: 11px;
                border: none;
                background: transparent;
                color: #666;
                cursor: pointer;
                border-radius: 3px;
                transition: all 0.15s ease;
            }

            .mcp-tuple-anyof-tabs .mcp-anyof-tab:hover {
                background: #e0e0e0;
            }

            .mcp-tuple-anyof-tabs .mcp-anyof-tab.active {
                background: white;
                color: #333;
                font-weight: 500;
                box-shadow: 0 1px 2px rgba(0,0,0,0.1);
            }

            .mcp-tuple-multitype-input {
                min-height: 32px;
                display: flex;
                align-items: center;
            }

            .mcp-tuple-multitype-input input[type="checkbox"] {
                margin: 0;
            }
        `;

        document.head.appendChild(style);
    }
}

// Export for use
if (typeof module !== 'undefined' && module.exports) {
    module.exports = MCPSchemaUIGenerator;
}